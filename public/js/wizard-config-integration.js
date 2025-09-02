// public/js/wizard-config-integration.js
// Client-side integration for your existing ezHealthKonnect wizard
// OPTIMIZED VERSION with enhanced error handling and best practices

/**
 * Interface Configuration Manager
 * Works with your existing Node.js app and Go backend proxy
 */
class InterfaceConfigManager {
    constructor(wizard) {
        this.wizard = wizard;
        this.nodeBackendUrl = window.location.origin;
        this.requestTimeout = 30000; // 30 second timeout
        this.retryAttempts = 3;
    }

    /**
     * Save configuration after Step 4 completion
     * Calls your new /api/wizard/save-config endpoint
     */
    async saveConfigurationAfterStep4() {
        try {
            console.log('📄 Starting interface configuration save...');
            
            // Collect configuration from wizard
            const wizardData = this.collectWizardConfiguration();
            
            // Validate required data
            if (!this.validateConfiguration(wizardData)) {
                throw new Error('Configuration validation failed');
            }
            
            // Show progress indicator
            this.showLoading('Saving Configuration...', 'Please wait while we save your interface configuration');
            
            // Send to your Node.js backend (which proxies to Go)
            const result = await this.sendToBackendWithRetry('/api/wizard/save-config', {
                wizardData: wizardData
            });
            
            // Store interface ID for later use
            if (this.wizard.wizardData && result.data?.interfaceId) {
                this.wizard.wizardData.interfaceId = result.data.interfaceId;
            }
            
            // Hide loading indicator
            this.hideLoading();
            
            // Show success notification
            this.showNotification('✅ Configuration saved successfully!', 'success');
            
            console.log('✅ Interface configuration saved:', result.data);
            return result;
            
        } catch (error) {
            console.error('❌ Failed to save configuration:', error);
            this.hideLoading();
            this.showNotification(`Failed to save configuration: ${error.message}`, 'error');
            throw error;
        }
    }

    /**
     * Complete wizard (save + activate in one call)
     */
    async completeWizard() {
        try {
            console.log('🏁 Completing wizard...');
            
            // Collect configuration from wizard
            const wizardData = this.collectWizardConfiguration();
            
            // Validate required data
            if (!this.validateConfiguration(wizardData)) {
                throw new Error('Configuration validation failed');
            }
            
            // Show progress indicator
            this.showLoading('Creating Interface...', 'Saving configuration and activating interface...');
            
            // Send complete request to your Node.js backend
            const result = await this.sendToBackendWithRetry('/api/wizard/complete', {
                wizardData: wizardData
            });
            
            // Store interface ID
            if (this.wizard.wizardData && result.data?.interfaceId) {
                this.wizard.wizardData.interfaceId = result.data.interfaceId;
            }
            
            // Hide loading indicator
            this.hideLoading();
            
            // Show success notification
            this.showNotification('🎉 Interface created and activated successfully!', 'success');
            
            console.log('✅ Wizard completed:', result.data);
            return result;
            
        } catch (error) {
            console.error('❌ Failed to complete wizard:', error);
            this.hideLoading();
            this.showNotification(`Failed to create interface: ${error.message}`, 'error');
            throw error;
        }
    }

    /**
     * Collect configuration data from wizard
     * Safely extracts data with fallbacks
     */
    collectWizardConfiguration() {
        try {
            const wizardData = this.wizard.wizardData || {};
            const currentData = this.wizard.getCurrentWizardData?.() || {};
            
            // Merge data sources
            const configuration = {
                // Step 1: Interface Details
                name: wizardData.name || currentData.name || 'Untitled Interface',
                description: wizardData.description || currentData.description || '',
                
                // Step 2: Source Configuration
                sourceType: wizardData.sourceType || currentData.sourceType,
                sourceConfig: wizardData.sourceConfig || currentData.sourceConfig || {},
                
                // Step 3: Target Configuration
                targetType: wizardData.targetType || currentData.targetType,
                targetConfig: wizardData.targetConfig || currentData.targetConfig || {},
                
                // Step 4: FHIR Mapping Configuration
                mappingRuleIds: wizardData.mappingRuleIds || currentData.mappingRuleIds || [],
                fhirVersion: wizardData.fhirVersion || currentData.fhirVersion || 'R4',
                fhirProfile: wizardData.fhirProfile || currentData.fhirProfile || 'base',
                createBundle: wizardData.createBundle || currentData.createBundle || false,
                
                // Additional metadata
                messageType: wizardData.messageType || currentData.messageType,
                processingRules: wizardData.processingRules || currentData.processingRules || {},
                transformationMapping: wizardData.transformationMapping || currentData.transformationMapping || {},
                
                // Timestamps
                createdAt: new Date().toISOString(),
                createdBy: this.getCurrentUser()
            };
            
            console.log('📊 Collected wizard configuration:', configuration);
            return configuration;
            
        } catch (error) {
            console.error('❌ Error collecting wizard configuration:', error);
            throw new Error(`Failed to collect wizard data: ${error.message}`);
        }
    }

    /**
     * Validate configuration before sending
     */
    validateConfiguration(config) {
        try {
            const errors = [];
            
            // Required fields validation
            if (!config.name || config.name.trim().length === 0) {
                errors.push('Interface name is required');
            }
            
            if (!config.sourceType) {
                errors.push('Source type is required');
            }
            
            if (!config.targetType) {
                errors.push('Target type is required');
            }
            
            // Log validation results
            if (errors.length > 0) {
                console.error('❌ Configuration validation failed:', errors);
                this.showNotification(`Validation failed: ${errors.join(', ')}`, 'error');
                return false;
            }
            
            console.log('✅ Configuration validation passed');
            return true;
            
        } catch (error) {
            console.error('❌ Error during validation:', error);
            return false;
        }
    }

    /**
     * Send request to backend with retry logic
     */
    async sendToBackendWithRetry(endpoint, data) {
        let lastError = null;
        
        for (let attempt = 1; attempt <= this.retryAttempts; attempt++) {
            try {
                console.log(`📡 Attempt ${attempt}/${this.retryAttempts} - Sending to ${endpoint}`);
                
                const controller = new AbortController();
                const timeoutId = setTimeout(() => controller.abort(), this.requestTimeout);
                
                const response = await fetch(`${this.nodeBackendUrl}${endpoint}`, {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                    },
                    body: JSON.stringify(data),
                    signal: controller.signal
                });
                
                clearTimeout(timeoutId);
                
                if (!response.ok) {
                    throw new Error(`HTTP ${response.status}: ${response.statusText}`);
                }
                
                const result = await response.json();
                console.log(`✅ Request successful on attempt ${attempt}`);
                return result;
                
            } catch (error) {
                lastError = error;
                console.warn(`⚠️ Attempt ${attempt} failed:`, error.message);
                
                if (attempt < this.retryAttempts) {
                    const delay = Math.pow(2, attempt) * 1000; // Exponential backoff
                    console.log(`⏳ Retrying in ${delay}ms...`);
                    await new Promise(resolve => setTimeout(resolve, delay));
                }
            }
        }
        
        throw new Error(`All ${this.retryAttempts} attempts failed. Last error: ${lastError.message}`);
    }

    /**
     * Show loading indicator
     */
    showLoading(title, message) {
        try {
            const overlay = document.getElementById('loadingOverlay');
            const titleEl = document.getElementById('loadingText');
            const messageEl = document.getElementById('loadingSubtext');
            
            if (overlay) {
                overlay.style.display = 'flex';
                if (titleEl) titleEl.textContent = title;
                if (messageEl) messageEl.textContent = message;
            }
        } catch (error) {
            console.warn('Could not show loading indicator:', error);
        }
    }

    /**
     * Hide loading indicator
     */
    hideLoading() {
        try {
            const overlay = document.getElementById('loadingOverlay');
            if (overlay) {
                overlay.style.display = 'none';
            }
        } catch (error) {
            console.warn('Could not hide loading indicator:', error);
        }
    }

    /**
     * Show notification to user
     */
    showNotification(message, type = 'info') {
        try {
            // Try to use existing notification system
            if (window.showNotification) {
                window.showNotification(message, type);
                return;
            }
            
            // Fallback notification
            console.log(`📢 ${type.toUpperCase()}: ${message}`);
            
            // Create a simple toast notification
            const toast = document.createElement('div');
            toast.className = `toast toast-${type}`;
            toast.textContent = message;
            toast.style.cssText = `
                position: fixed;
                top: 20px;
                right: 20px;
                background: ${type === 'success' ? '#4CAF50' : type === 'error' ? '#f44336' : '#2196F3'};
                color: white;
                padding: 12px 20px;
                border-radius: 4px;
                z-index: 10000;
                max-width: 400px;
                word-wrap: break-word;
            `;
            
            document.body.appendChild(toast);
            
            setTimeout(() => {
                if (toast.parentNode) {
                    toast.parentNode.removeChild(toast);
                }
            }, 5000);
            
        } catch (error) {
            console.warn('Could not show notification:', error);
        }
    }

    /**
     * Get current user info
     */
    getCurrentUser() {
        try {
            // Try to get user from various sources
            return (
                window.currentUser?.email ||
                window.user?.email ||
                localStorage.getItem('userEmail') ||
                'system'
            );
        } catch (error) {
            return 'system';
        }
    }
}

/**
 * Enhanced Step 4 Handler
 * Integrates with your existing Step 4 functionality
 */
class EnhancedStep4Handler {
    constructor(wizard) {
        this.wizard = wizard;
        this.configManager = new InterfaceConfigManager(wizard);
    }

    /**
     * Called when step 4 mapping is completed successfully
     */
    async onMappingCompleted(mappingData) {
        try {
            console.log('🎯 Step 4 mapping completed, saving configuration...');
            
            // Store mapping data in wizard safely
            this.wizard.wizardData = this.wizard.wizardData || {};
            this.wizard.wizardData.mappingRuleIds = mappingData?.mappingRuleIds || [];
            this.wizard.wizardData.fhirVersion = mappingData?.fhirVersion || 'R4';
            this.wizard.wizardData.fhirProfile = mappingData?.fhirProfile || 'base';
            this.wizard.wizardData.createBundle = mappingData?.createBundle || false;
            
            // Save configuration
            await this.configManager.saveConfigurationAfterStep4();
            
            console.log('✅ Configuration saved successfully after step 4');
            
        } catch (error) {
            console.error('❌ Failed to save configuration after step 4:', error);
            // Don't block progression to step 5, but show warning
            this.configManager.showNotification('Warning: Configuration not saved. You can retry from the dashboard.', 'warning');
        }
    }

    /**
     * Initialize step 4 enhancements
     */
    initialize() {
        console.log('🔧 Initializing enhanced Step 4 handler...');
        console.log('✅ Enhanced Step 4 handler initialized');
    }
}

/**
 * Enhanced Wizard Main that integrates with your existing setup
 */
class EnhancedWizardMain {
    constructor(existingWizard) {
        try {
            // If you have an existing wizard, enhance it
            if (existingWizard) {
                this.wizard = existingWizard;
                this.enhanceExistingWizard();
            } else {
                // Create new wizard functionality
                this.wizard = this;
                this.initializeWizard();
            }
            
            this.configManager = new InterfaceConfigManager(this.wizard);
            this.setupConfigurationHandlers();
            
        } catch (error) {
            console.error('❌ Error initializing EnhancedWizardMain:', error);
        }
    }

    enhanceExistingWizard() {
        console.log('🔧 Enhancing existing wizard with configuration management...');
        
        try {
            // Add configuration management to existing wizard
            this.wizard.configManager = new InterfaceConfigManager(this.wizard);
            
            // Enhance existing finishWizard method if it exists
            if (this.wizard.finishWizard) {
                const originalFinish = this.wizard.finishWizard.bind(this.wizard);
                this.wizard.finishWizard = async () => {
                    try {
                        console.log('🎯 Enhanced finishWizard called - attempting to save and activate...');
                        await this.configManager.completeWizard();
                    } catch (error) {
                        console.error('Enhanced finish failed, trying original:', error);
                        return originalFinish();
                    }
                };
            }
            
            // Enhance existing step handlers for Step 4
            if (this.wizard.stepHandlers?.['4']) {
                const originalHandler = this.wizard.stepHandlers['4'];
                
                // Store original methods
                if (originalHandler.onStepComplete) {
                    this.originalOnStepComplete = originalHandler.onStepComplete.bind(originalHandler);
                    
                    // Override with enhanced version
                    originalHandler.onStepComplete = async (stepNumber) => {
                        try {
                            console.log('🔧 Enhanced Step 4 completion handler...');
                            
                            // Try to save configuration first
                            const step4Handler = new EnhancedStep4Handler(this.wizard);
                            await step4Handler.onMappingCompleted(this.wizard.wizardData);
                            
                        } catch (error) {
                            console.error('❌ Enhanced step completion failed:', error);
                        } finally {
                            // Call original completion if it exists
                            if (this.originalOnStepComplete) {
                                return this.originalOnStepComplete(stepNumber);
                            }
                        }
                    };
                }
            }
        } catch (error) {
            console.error('❌ Error enhancing Step 4 handler:', error);
        }
    }

    initializeWizard() {
        console.log('🔧 Initializing new enhanced wizard...');
        
        try {
            // Initialize basic wizard properties
            this.currentStep = 1;
            this.totalSteps = 5;
            this.wizardData = {};
            
            console.log('✅ New wizard initialized');
        } catch (error) {
            console.error('❌ Error initializing new wizard:', error);
        }
    }

    setupConfigurationHandlers() {
        try {
            // Set up step 4 handler
            this.step4Handler = new EnhancedStep4Handler(this.wizard);
            this.step4Handler.initialize();
            
            console.log('✅ Enhanced wizard configuration handlers setup complete');
        } catch (error) {
            console.error('❌ Error setting up configuration handlers:', error);
        }
    }

    /**
     * Enhanced finish wizard with complete flow
     */
    async finishWizard() {
        try {
            console.log('🏁 Enhanced wizard finish...');
            
            // Use complete wizard flow (save + activate in one call)
            await this.configManager.completeWizard();
            
        } catch (error) {
            console.error('❌ Enhanced wizard finish failed:', error);
            throw error;
        }
    }
}

/**
 * Initialize wizard enhancement
 * Works with your existing wizard setup
 */
function initializeWizardEnhancement() {
    try {
        console.log('🚀 Initializing wizard configuration integration...');
        
        // Strategy 1: Look for existing wizard instance
        if (window.wizard) {
            console.log('📦 Found existing wizard instance, enhancing...');
            window.enhancedWizard = new EnhancedWizardMain(window.wizard);
            console.log('✅ Wizard configuration integration initialized');
            return;
        }
        
        // Strategy 2: Wait for wizard to be created
        let attempts = 0;
        const maxAttempts = 25; // 5 seconds max wait
        const checkInterval = setInterval(() => {
            attempts++;
            if (window.wizard) {
                clearInterval(checkInterval);
                console.log('📦 Wizard found after waiting, enhancing...');
                window.enhancedWizard = new EnhancedWizardMain(window.wizard);
                console.log('✅ Wizard configuration integration initialized after wait');
            } else if (attempts >= maxAttempts) {
                clearInterval(checkInterval);
                console.log('⏰ Timeout waiting for wizard instance');
            }
        }, 200);
        
        return;
        
        // Strategy 3: Listen for wizard ready event
        window.addEventListener('wizardReady', function(event) {
            console.log('📻 Received wizardReady event, enhancing wizard...');
            const wizard = event.detail?.wizard || window.wizard;
            if (wizard) {
                window.enhancedWizard = new EnhancedWizardMain(wizard);
                console.log('✅ Wizard configuration integration initialized via event');
            }
        });
        
        console.log('ℹ️ No wizard found on this page yet, listeners set up for future detection');
        
    } catch (error) {
        console.error('❌ Error during wizard enhancement initialization:', error);
    }
}

// Auto-initialize when DOM is ready
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initializeWizardEnhancement);
} else {
    // DOM already loaded, try to initialize immediately
    initializeWizardEnhancement();
}

// Also try after a short delay to catch late-loading wizards
setTimeout(initializeWizardEnhancement, 1000);

// Export for manual initialization
window.InterfaceConfigManager = InterfaceConfigManager;
window.EnhancedWizardMain = EnhancedWizardMain;

// For CommonJS environments
if (typeof module !== 'undefined' && module.exports) {
    module.exports = {
        InterfaceConfigManager,
        EnhancedStep4Handler,
        EnhancedWizardMain
    };
}

console.log('✅ Wizard Configuration Integration loaded and ready');