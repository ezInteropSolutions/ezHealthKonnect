// public/js/wizard/wizard-config-integration.js
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
            
            console.log('✅ Configuration validation passed');
            
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
            
            console.log('✅ Wizard completed successfully:', result.data);

            return result;
            
        } catch (error) {
            console.error('❌ Failed to complete wizard:', error);
            this.hideLoading();
            this.showNotification(`Failed to complete wizard: ${error.message}`, 'error');
            throw error;
        }
    }

    /**
     * Collect configuration data from all wizard steps
     * ✅ ARCHITECTURE: Format + Connectivity separation - backend handles mapping
     */
    collectWizardConfiguration() {
        console.log('📋 Collecting wizard configuration data...');
        
        try {
            // Get raw configuration values - backend will map to format + connectivity
            const rawConfig = {
                // Step 1: Basic Configuration (UI values)
                name: this.getElementValue('wizardInterfaceName'),
                description: this.getElementValue('wizardInterfaceDescription'),
                sourceType: this.getElementValue('wizardSourceType'),  // UI: hl7v2, file, http, etc.
                targetType: this.getElementValue('wizardTargetType'),  // UI: fhir, database, file, etc.
                
                // Step 2: Source Configuration
                sourceSettings: {
                    host: this.getElementValue('sourceHost'),
                    port: this.getElementValue('sourcePort'),
                    path: this.getElementValue('sourcePath'),
                    username: this.getElementValue('sourceUsername'),
                    password: this.getElementValue('sourcePassword'),
                    timeout: this.getElementValue('sourceTimeout') || 30,
                    ssl: this.getElementChecked('sourceSSL'),
                    authMethod: this.getElementValue('sourceAuthMethod') || 'none'
                },
                
                // Step 3: Target Configuration
                targetSettings: {
                    host: this.getElementValue('targetHost'),
                    port: this.getElementValue('targetPort'),
                    path: this.getElementValue('targetPath'),
                    username: this.getElementValue('targetUsername'),
                    password: this.getElementValue('targetPassword'),
                    timeout: this.getElementValue('targetTimeout') || 30,
                    ssl: this.getElementChecked('targetSSL'),
                    authMethod: this.getElementValue('targetAuthMethod') || 'none',
                    database: this.getElementValue('targetDatabase'),
                    table: this.getElementValue('targetTable')
                },
                
                // Step 4: Mapping Data (from wizard state)
                mappingConfiguration: {
                    mappingRuleIds: this.wizard.wizardData?.mappingRuleIds || [],
                    fhirVersion: this.wizard.wizardData?.fhirVersion || 'R4',
                    fhirProfile: this.wizard.wizardData?.fhirProfile || 'base',
                    createBundle: this.wizard.wizardData?.createBundle || false,
                    resourceOverrides: this.wizard.wizardData?.resourceOverrides || {}
                }
            };

            // Remove empty values to avoid backend validation issues
            const cleanConfig = this.removeEmptyValues(rawConfig);
            
            console.log('📋 Collected configuration:', cleanConfig);
            return cleanConfig;
            
        } catch (error) {
            console.error('❌ Error collecting wizard configuration:', error);
            throw new Error('Failed to collect wizard configuration data');
        }
    }

    /**
     * Validate required configuration fields
     */
    validateConfiguration(config) {
        console.log('🔍 Validating configuration...');
        
        const requiredFields = ['name', 'sourceType', 'targetType'];
        
        for (const field of requiredFields) {
            if (!config[field] || config[field].trim() === '') {
                console.error(`❌ Validation failed: ${field} is required`);
                this.showNotification(`Missing required field: ${field}`, 'error');
                return false;
            }
        }
        
        // Additional validation
        if (config.name.length > 255) {
            console.error('❌ Validation failed: Interface name too long');
            this.showNotification('Interface name must be less than 255 characters', 'error');
            return false;
        }
        
        console.log('✅ Configuration validation passed');
        return true;
    }

    /**
     * Send request to Node.js backend with retry logic
     */
    async sendToBackendWithRetry(endpoint, data, attempt = 1) {
        try {
            console.log(`📡 Attempt ${attempt}/${this.retryAttempts} - Sending to ${endpoint}`);
            return await this.sendToBackend(endpoint, data);
        } catch (error) {
            if (attempt < this.retryAttempts && this.isRetryableError(error)) {
                console.warn(`⚠️ Request failed (attempt ${attempt}/${this.retryAttempts}), retrying...`, error.message);
                await this.delay(1000 * attempt); // Progressive delay
                return this.sendToBackendWithRetry(endpoint, data, attempt + 1);
            }
            throw error;
        }
    }

    /**
     * Send request to your Node.js backend
     */
    async sendToBackend(endpoint, data) {
        const controller = new AbortController();
        const timeoutId = setTimeout(() => controller.abort(), this.requestTimeout);
        
        try {
            const response = await fetch(endpoint, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Accept': 'application/json'
                },
                credentials: 'same-origin', // Include session cookies
                body: JSON.stringify(data),
                signal: controller.signal
            });

            clearTimeout(timeoutId);
            
            // FIXED: Log success AFTER the request completes
            console.log(`✅ Request successful on attempt ${data.attempt || 1}`);

            if (!response.ok) {
                const errorText = await response.text();
                throw new Error(`HTTP ${response.status}: ${errorText}`);
            }

            const result = await response.json();
            
            if (!result.success) {
                throw new Error(result.error || 'Unknown backend error');
            }

            return result;

        } catch (error) {
            clearTimeout(timeoutId);
            
            if (error.name === 'AbortError') {
                throw new Error('Request timeout - please try again');
            }
            
            throw error;
        }
    }

    /**
     * Helper methods
     */
    getElementValue(id) {
        const element = document.getElementById(id);
        return element ? element.value : '';
    }

    getElementChecked(id) {
        const element = document.getElementById(id);
        return element ? element.checked : false;
    }

    removeEmptyValues(obj) {
        const cleaned = {};
        for (const [key, value] of Object.entries(obj)) {
            if (value !== null && value !== undefined && value !== '') {
                if (typeof value === 'object' && !Array.isArray(value)) {
                    const cleanedNested = this.removeEmptyValues(value);
                    if (Object.keys(cleanedNested).length > 0) {
                        cleaned[key] = cleanedNested;
                    }
                } else {
                    cleaned[key] = value;
                }
            }
        }
        return cleaned;
    }

    isRetryableError(error) {
        return error.message.includes('timeout') || 
               error.message.includes('network') ||
               error.message.includes('Failed to fetch');
    }

    delay(ms) {
        return new Promise(resolve => setTimeout(resolve, ms));
    }

    showLoading(title, message) {
        if (this.wizard.showLoading) {
            this.wizard.showLoading(title, message);
        }
    }

    hideLoading() {
        if (this.wizard.hideLoading) {
            this.wizard.hideLoading();
        }
    }

    showNotification(message, type) {
        if (this.wizard.showNotification) {
            this.wizard.showNotification(message, type);
        } else {
            console.log(`${type.toUpperCase()}: ${message}`);
        }
    }
}

/**
 * Enhanced Step 4 Handler - Called when mapping is completed
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
            
            // FIXED: Enhanced finishWizard method with auto-refresh protection and comprehensive modal closing
            if (this.wizard.finishWizard) {
                const originalFinish = this.wizard.finishWizard.bind(this.wizard);
                this.wizard.finishWizard = async () => {
                    try {
                        console.log('🎯 Enhanced finishWizard called - attempting to save and activate...');
                        
                        // STEP 1: STOP all auto-refresh to prevent interference
                        if (window.stopAutoRefresh) {
                            window.stopAutoRefresh();
                            console.log('🛑 Auto-refresh stopped to prevent modal interference');
                        }
                        
                        // STEP 2: Complete the wizard
                        await this.configManager.completeWizard();
                        
                        // STEP 3: FORCE close modal IMMEDIATELY with comprehensive cleanup
                        console.log('🚪 Force closing modal after completion');
                        
                        // Find and close the modal
                        const overlay = document.getElementById('wizardModalOverlay');
                        if (overlay) {
                            // Remove all possible classes
                            overlay.classList.remove('show', 'maximized', 'active', 'open', 'visible');
                            
                            // Force hide with multiple methods
                            overlay.style.display = 'none !important';
                            overlay.style.visibility = 'hidden !important';
                            overlay.style.opacity = '0 !important';
                            overlay.style.pointerEvents = 'none !important';
                            overlay.style.zIndex = '-1 !important';
                            
                            // Reset body
                            document.body.style.overflow = '';
                            document.body.style.paddingRight = '';
                            document.body.classList.remove('modal-open', 'wizard-open');
                            
                            console.log('✅ Modal force hidden with comprehensive styles');
                        }
                        
                        // STEP 4: Reset ALL wizard states
                        if (window.wizardNavigation) {
                            window.wizardNavigation.isModalOpen = false;
                            window.wizardNavigation.isFinishing = false;
                            window.wizardNavigation.currentStep = 1;
                        }
                        
                        if (window.wizard) {
                            window.wizard.currentStep = 1;
                            if (typeof window.wizard.resetWizard === 'function') {
                                window.wizard.resetWizard();
                            }
                        }
                        
                        // STEP 5: Clear session storage
                        try {
                            sessionStorage.removeItem('wizardData');
                            sessionStorage.removeItem('wizardStep');
                            sessionStorage.removeItem('wizardProgress');
                        } catch (e) {
                            // Ignore storage errors
                        }
                        
                        // STEP 6: Wait a moment then restart auto-refresh and reload interfaces
                        setTimeout(() => {
                            console.log('🔄 Restarting auto-refresh and reloading interfaces');
                            
                            // Restart auto-refresh
                            if (window.startAutoRefresh) {
                                window.startAutoRefresh();
                            }
                            
                            // Reload interfaces to show the new one
                            if (typeof window.loadInterfaces === 'function') {
                                window.loadInterfaces();
                            }
                            
                            console.log('✅ Wizard cleanup complete - modal should be closed');
                        }, 500);
                        
                    } catch (error) {
                        console.error('Enhanced finish failed, trying original:', error);
                        
                        // Still try to close modal even if there's an error
                        const overlay = document.getElementById('wizardModalOverlay');
                        if (overlay) {
                            overlay.style.display = 'none !important';
                        }
                        
                        // Restart auto-refresh even on error
                        if (window.startAutoRefresh) {
                            window.startAutoRefresh();
                        }
                        
                        return originalFinish();
                    }
                };
            }
            
            // Enhance existing step handlers for Step 4
            if (this.wizard.stepHandlers?.code_summary) {
                const originalStepHandler = this.wizard.stepHandlers.code_summary;
                if (originalStepHandler && originalStepHandler.onStepComplete) {
                    this.originalOnStepComplete = originalStepHandler.onStepComplete.bind(originalStepHandler);
                    originalStepHandler.onStepComplete = async (stepNumber) => {
                        if (stepNumber === 4) {
                            // Enhanced Step 4 completion
                            await this.step4Handler.onMappingCompleted(this.wizard.wizardData);
                        }
                        // Call original completion if it exists
                        if (this.originalOnStepComplete) {
                            return this.originalOnStepComplete(stepNumber);
                        }
                    };
                }
            }
            
            console.log('✅ Enhanced wizard configuration handlers setup complete');
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
     * Helper methods delegated to configManager
     */
    collectWizardConfiguration() {
        return this.configManager.collectWizardConfiguration();
    }

    validateConfiguration(config) {
        return this.configManager.validateConfiguration(config);
    }

    showLoading(message) {
        if (this.wizard && this.wizard.showLoading) {
            this.wizard.showLoading(message);
        }
    }

    hideLoading() {
        if (this.wizard && this.wizard.hideLoading) {
            this.wizard.hideLoading();
        }
    }

    showNotification(message, type) {
        if (this.wizard && this.wizard.showNotification) {
            this.wizard.showNotification(message, type);
        } else {
            console.log(`${type.toUpperCase()}: ${message}`);
        }
    }
}

// =====================================
// INITIALIZATION WITH BETTER DETECTION
// =====================================

// Wait for wizard to be ready and then enhance it
function initializeWizardEnhancement() {
    console.log('🚀 Initializing wizard configuration integration...');
    
    try {
        // Strategy 1: Check for existing wizard instance
        if (window.wizard) {
            console.log('📦 Found existing wizard instance, enhancing...');
            window.enhancedWizardMain = new EnhancedWizardMain(window.wizard);
            console.log('✅ Wizard configuration integration initialized');
            return;
        }
        
        // Strategy 2: Check for wizard container
        if (document.getElementById('wizardModalOverlay') || document.querySelector('.wizard-modal-overlay')) {
            console.log('🎯 Found wizard container, waiting for wizard instance...');
            
            // Wait for window.wizard to be created
            let attempts = 0;
            const checkInterval = setInterval(() => {
                attempts++;
                
                if (window.wizard) {
                    clearInterval(checkInterval);
                    console.log('📦 Wizard found after waiting, enhancing...');
                    window.enhancedWizardMain = new EnhancedWizardMain(window.wizard);
                    console.log('✅ Wizard configuration integration initialized after wait');
                } else if (attempts > 50) { // 10 seconds max wait
                    clearInterval(checkInterval);
                    console.log('⏰ Timeout waiting for wizard instance');
                }
            }, 200);
            
            return;
        }
        
        // Strategy 3: Listen for wizard ready event
        window.addEventListener('wizardReady', function(event) {
            console.log('📻 Received wizardReady event, enhancing wizard...');
            const wizard = event.detail?.wizard || window.wizard;
            if (wizard) {
                window.enhancedWizardMain = new EnhancedWizardMain(wizard);
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

// Add CSS for smooth modal transitions
const additionalCSS = `
.wizard-modal-overlay {
    transition: opacity 0.3s ease, visibility 0.3s ease !important;
}

.wizard-modal-overlay:not(.show) {
    opacity: 0 !important;
    visibility: hidden !important;
    pointer-events: none !important;
}

.wizard-btn.disabled {
    opacity: 0.6;
    cursor: not-allowed !important;
}

.spinner {
    display: inline-block;
    width: 12px;
    height: 12px;
    border: 2px solid #f3f3f3;
    border-top: 2px solid #3498db;
    border-radius: 50%;
    animation: spin 1s linear infinite;
    margin-right: 5px;
}

@keyframes spin {
    0% { transform: rotate(0deg); }
    100% { transform: rotate(360deg); }
}
`;

// Inject the CSS
const style = document.createElement('style');
style.id = 'wizard-close-spinner-styles';
style.textContent = additionalCSS;
document.head.appendChild(style);

// Enhanced debugging for wizard close mechanisms
window.debugWizardClose = function() {
    console.log('🔧 Debug: Testing wizard close mechanisms...');
    
    console.log('Available close functions:');
    console.log('- window.closeWizard:', typeof window.closeWizard);
    console.log('- window.wizardNavigation.closeWizard:', window.wizardNavigation ? typeof window.wizardNavigation.closeWizard : 'N/A');
    console.log('- window.wizard.closeModal:', window.wizard ? typeof window.wizard.closeModal : 'N/A');
    console.log('- window.enhancedWizardMain.closeWizardModal:', window.enhancedWizardMain ? typeof window.enhancedWizardMain.closeWizardModal : 'N/A');
    
    // Test direct DOM access
    const overlay = document.getElementById('wizardModalOverlay');
    console.log('- Modal overlay found:', !!overlay);
    if (overlay) {
        console.log('- Modal classes:', overlay.className);
        console.log('- Modal display:', overlay.style.display);
    }
    
    // Attempt close via enhanced wizard
    if (window.enhancedWizardMain && typeof window.enhancedWizardMain.closeWizardModal === 'function') {
        console.log('Attempting enhanced close...');
        window.enhancedWizardMain.closeWizardModal();
    } else {
        console.log('Enhanced wizard main not available, using direct DOM manipulation...');
        if (overlay) {
            overlay.style.display = 'none !important';
            overlay.classList.remove('show', 'active', 'maximized');
            document.body.style.overflow = '';
        }
    }
};

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