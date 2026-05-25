// public/js/step4-complete-integration.js
// Integration helper for Step 4 completion with enhanced error handling

(function() {
    'use strict';
    
    console.log('🔄 Loading Step 4 Complete Integration...');
    
    /**
     * Step 4 Complete Integration Class
     * Handles the completion of Step 4 with proper error handling and dependency checking
     */
    class Step4CompleteIntegration {
        constructor() {
            this.initialized = false;
            this.retryAttempts = 0;
            this.maxRetries = 10;
            this.retryDelay = 1000; // 1 second
        }

        /**
         * Initialize the integration with proper dependency checking
         */
        async initialize() {
            try {
                console.log('🔧 Initializing Step 4 Complete Integration...');
                
                // Check if InterfaceConfigManager is available
                if (!window.InterfaceConfigManager) {
                    throw new Error('InterfaceConfigManager not available');
                }

                // Check if wizard instance exists
                const wizard = window.wizard || window.interfaceWizard;
                if (!wizard) {
                    throw new Error('Wizard instance not found');
                }

                console.log('✅ Dependencies found, creating config manager...');
                
                // Create the configuration manager instance
                this.configManager = new window.InterfaceConfigManager(wizard);
                
                console.log('✅ InterfaceConfigManager initialized successfully');
                
                // Set up step 4 completion handlers
                this.setupStep4CompletionHandlers(wizard);
                
                this.initialized = true;
                console.log('✅ Step 4 Complete Integration initialized successfully');
                
            } catch (error) {
                console.log(`❌ Failed to initialize Step 4 Complete Integration: ${error.message}`);
                
                // Retry logic for dependencies that haven't loaded yet
                if (this.retryAttempts < this.maxRetries) {
                    this.retryAttempts++;
                    console.log(`🔄 Retry attempt ${this.retryAttempts}/${this.maxRetries} in ${this.retryDelay}ms...`);
                    setTimeout(() => this.initialize(), this.retryDelay);
                } else {
                    console.error('❌ Max retries exceeded, Step 4 Complete Integration failed to initialize');
                }
            }
        }

        /**
         * Set up handlers for Step 4 completion events
         */
        setupStep4CompletionHandlers(wizard) {
            try {
                console.log('🔧 Setting up Step 4 completion handlers...');
                
                // Method 1: Enhance existing Step 4 handler
                if (wizard.stepHandlers && wizard.stepHandlers['4']) {
                    this.enhanceStep4Handler(wizard.stepHandlers['4']);
                }
                
                // Method 2: Listen for step completion events
                this.setupStepCompletionListener(wizard);
                
                // Method 3: Monitor for Step 4 mapping completion
                this.setupMappingCompletionMonitor(wizard);
                
                console.log('✅ Step 4 completion handlers set up successfully');
                
            } catch (error) {
                console.error('❌ Error setting up Step 4 completion handlers:', error);
            }
        }

        /**
         * Enhance existing Step 4 handler with configuration saving
         */
        enhanceStep4Handler(step4Handler) {
            try {
                console.log('🔧 Enhancing Step 4 handler...');
                
                // Store reference to original completion method
                if (step4Handler.onStepComplete) {
                    const originalOnStepComplete = step4Handler.onStepComplete.bind(step4Handler);
                    
                    // Override with enhanced version
                    step4Handler.onStepComplete = async (stepNumber) => {
                        console.log('🎯 Enhanced Step 4 completion handler triggered');
                        
                        try {
                            // Save configuration if this is step 4
                            if (stepNumber === 4) {
                                await this.handleStep4Completion();
                            }
                        } catch (error) {
                            console.warn('⚠️ Configuration save failed, but continuing:', error);
                        } finally {
                            // Always call original handler
                            if (originalOnStepComplete) {
                                return originalOnStepComplete(stepNumber);
                            }
                        }
                    };
                }
                
                console.log('✅ Step 4 handler enhanced successfully');
                
            } catch (error) {
                console.error('❌ Error enhancing Step 4 handler:', error);
            }
        }

        /**
         * Set up listener for step completion events
         */
        setupStepCompletionListener(wizard) {
            try {
                // Listen for custom step completion events
                document.addEventListener('stepCompleted', (event) => {
                    if (event.detail && event.detail.stepNumber === 4) {
                        console.log('📻 Step 4 completion event received');
                        this.handleStep4Completion();
                    }
                });
                
                // Listen for wizard events if available
                if (wizard.on && typeof wizard.on === 'function') {
                    wizard.on('stepCompleted', (stepNumber) => {
                        if (stepNumber === 4) {
                            console.log('📻 Wizard Step 4 completion event received');
                            this.handleStep4Completion();
                        }
                    });
                }
                
                console.log('✅ Step completion listeners set up');
                
            } catch (error) {
                console.error('❌ Error setting up step completion listeners:', error);
            }
        }

        /**
         * Monitor for Step 4 mapping completion
         */
        setupMappingCompletionMonitor(wizard) {
            try {
                // Monitor wizard data changes for step 4
                if (wizard.wizardData) {
                    this.lastMappingData = JSON.stringify(wizard.wizardData.mappingRuleIds || []);
                    
                    // Periodically check for changes
                    this.mappingMonitorInterval = setInterval(() => {
                        try {
                            const currentMappingData = JSON.stringify(wizard.wizardData.mappingRuleIds || []);
                            if (currentMappingData !== this.lastMappingData && wizard.currentStep === 4) {
                                console.log('🔍 Step 4 mapping data changed, checking completion...');
                                this.lastMappingData = currentMappingData;
                                
                                // If we have mapping rules, consider step 4 potentially complete
                                if (wizard.wizardData.mappingRuleIds && wizard.wizardData.mappingRuleIds.length > 0) {
                                    setTimeout(() => this.handleStep4Completion(), 500); // Small delay for UI updates
                                }
                            }
                        } catch (error) {
                            console.warn('⚠️ Error in mapping monitor:', error);
                        }
                    }, 2000); // Check every 2 seconds
                }
                
                console.log('✅ Mapping completion monitor set up');
                
            } catch (error) {
                console.error('❌ Error setting up mapping completion monitor:', error);
            }
        }

        /**
         * Handle Step 4 completion
         */
        async handleStep4Completion() {
            try {
                console.log('🎯 Handling Step 4 completion...');
                
                if (!this.configManager) {
                    console.warn('⚠️ Config manager not available, skipping save');
                    return;
                }
                
                // Get wizard data
                const wizard = window.wizard || window.interfaceWizard;
                if (!wizard || !wizard.wizardData) {
                    console.warn('⚠️ Wizard data not available, skipping save');
                    return;
                }
                
                // Check if we have mapping data to save
                const hasMapping = wizard.wizardData.mappingRuleIds && wizard.wizardData.mappingRuleIds.length > 0;
                
                if (!hasMapping) {
                    console.log('ℹ️ No mapping data to save, skipping Step 4 configuration save');
                    return;
                }
                
                console.log('💾 Saving Step 4 configuration...');
                
                // Prepare mapping data
                const mappingData = {
                    mappingRuleIds: wizard.wizardData.mappingRuleIds,
                    fhirVersion: wizard.wizardData.fhirVersion || 'R4',
                    fhirProfile: wizard.wizardData.fhirProfile || 'base',
                    createBundle: wizard.wizardData.createBundle || false
                };
                
                // Save configuration via the config manager
                await this.configManager.saveConfigurationAfterStep4();
                
                console.log('✅ Step 4 configuration saved successfully');
                
            } catch (error) {
                console.error('❌ Error handling Step 4 completion:', error);
                
                // Show user-friendly notification
                if (window.showNotification) {
                    window.showNotification('Configuration save failed. You can retry from the dashboard.', 'warning');
                }
            }
        }

        /**
         * Clean up resources
         */
        cleanup() {
            try {
                if (this.mappingMonitorInterval) {
                    clearInterval(this.mappingMonitorInterval);
                    this.mappingMonitorInterval = null;
                }
                console.log('✅ Step 4 Complete Integration cleaned up');
            } catch (error) {
                console.error('❌ Error during cleanup:', error);
            }
        }
    }

    // Initialize the integration
    const integration = new Step4CompleteIntegration();
    
    // Wait for DOM to be ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', () => integration.initialize());
    } else {
        // DOM already ready, initialize immediately
        setTimeout(() => integration.initialize(), 100);
    }
    
    // Also initialize when wizard becomes available
    let wizardCheckAttempts = 0;
    const maxWizardCheckAttempts = 20;
    
    const checkForWizard = () => {
        wizardCheckAttempts++;
        
        if (window.wizard || window.interfaceWizard) {
            integration.initialize();
        } else if (wizardCheckAttempts < maxWizardCheckAttempts) {
            setTimeout(checkForWizard, 500);
        } else {
            console.log('⏰ Wizard not found after maximum attempts, Step 4 hooks may not be fully active');
        }
    };
    
    // Start checking for wizard
    setTimeout(checkForWizard, 1000);
    
    // Export for global access
    window.Step4CompleteIntegration = Step4CompleteIntegration;
    
    console.log('✅ Step 4 Complete Integration loaded');

})();