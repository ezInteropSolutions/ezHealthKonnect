// public/js/wizard-connectivity-enhancements.js
// Enhanced connectivity form handling for the EXISTING wizard Step 1

(function() {
    'use strict';
    
    console.log('Loading wizard connectivity enhancements for existing Step 1...');
    
    /**
     * Wizard Connectivity Manager
     * Works with the EXISTING Step 1 form elements
     */
    class WizardConnectivityManager {
        constructor(wizard) {
            this.wizard = wizard;
            this.initialized = false;
            this.connectivityDefaults = {
                source: {
                    'hl7v2': 'tcp',
                    'hl7v3': 'tcp', 
                    'hl7': 'tcp',
                    'fhir': 'http',
                    'ccda': 'file',
                    'file': 'file',
                    'database': 'database'
                },
                target: {
                    'fhir': 'http',
                    'hl7v2': 'tcp',
                    'hl7v3': 'tcp',
                    'ccda': 'file',
                    'database': 'database',
                    'flatfile': 'file',
                    'json': 'file',
                    'xml': 'file'
                }
            };
        }

        /**
         * Initialize connectivity enhancements
         */
        initialize() {
            try {
                console.log('Initializing wizard connectivity manager for existing form...');
                
                // Set up handlers multiple ways to catch the elements when they're ready
                this.setupMultipleInitStrategies();
                
            } catch (error) {
                console.error('Error initializing connectivity manager:', error);
            }
        }

        /**
         * Multiple strategies to ensure we catch the elements when they're ready
         */
        setupMultipleInitStrategies() {
            // Strategy 1: Immediate setup if DOM ready
            if (document.readyState !== 'loading') {
                setTimeout(() => this.setupConnectivityHandlers(), 100);
            }
            
            // Strategy 2: DOM content loaded event
            document.addEventListener('DOMContentLoaded', () => {
                setTimeout(() => this.setupConnectivityHandlers(), 100);
            });
            
            // Strategy 3: When wizard modal opens
            this.setupWizardModalHandlers();
            
            // Strategy 4: Periodic retry for up to 30 seconds
            this.setupPeriodicRetry();
        }

        /**
         * Set up handlers for wizard modal events
         */
        setupWizardModalHandlers() {
            // Listen for modal open events
            document.addEventListener('click', (event) => {
                // Check if this is opening the wizard modal
                if (event.target.matches('[onclick*="openInterfaceWizard"]') || 
                    event.target.closest('[onclick*="openInterfaceWizard"]')) {
                    console.log('Wizard modal opening detected, setting up connectivity...');
                    setTimeout(() => this.setupConnectivityHandlers(), 300);
                }
            });
            
            // Watch for modal overlay becoming visible
            const observer = new MutationObserver((mutations) => {
                mutations.forEach((mutation) => {
                    if (mutation.type === 'attributes' && 
                        mutation.attributeName === 'class' &&
                        mutation.target.id === 'wizardModalOverlay') {
                        if (mutation.target.classList.contains('show')) {
                            console.log('Wizard modal shown, setting up connectivity...');
                            setTimeout(() => this.setupConnectivityHandlers(), 200);
                        }
                    }
                });
            });
            
            const modalOverlay = document.getElementById('wizardModalOverlay');
            if (modalOverlay) {
                observer.observe(modalOverlay, { attributes: true });
            }
        }

        /**
         * Periodic retry to catch elements when they appear
         */
        setupPeriodicRetry() {
            let retryCount = 0;
            const maxRetries = 60; // 30 seconds with 500ms interval
            
            const retryInterval = setInterval(() => {
                retryCount++;
                
                if (this.areWizardElementsReady() && !this.initialized) {
                    console.log(`Connectivity elements found on retry ${retryCount}`);
                    this.setupConnectivityHandlers();
                    clearInterval(retryInterval);
                } else if (retryCount >= maxRetries) {
                    console.log('Max retries reached for connectivity setup');
                    clearInterval(retryInterval);
                }
            }, 500);
        }

        /**
         * Check if required wizard elements are present and visible
         */
        areWizardElementsReady() {
            const sourceType = document.getElementById('wizardSourceType');
            const targetType = document.getElementById('wizardTargetType');
            const sourceConnectivity = document.getElementById('wizardSourceConnectivity');
            const targetConnectivity = document.getElementById('wizardTargetConnectivity');
            
            return sourceType && targetType && sourceConnectivity && targetConnectivity &&
                   sourceType.offsetParent !== null; // Check if visible
        }

        /**
         * Set up connectivity form handlers for EXISTING elements
         */
        setupConnectivityHandlers() {
            try {
                if (this.initialized) {
                    console.log('Connectivity handlers already initialized');
                    return;
                }

                console.log('Setting up connectivity form handlers for existing elements...');
                
                const sourceType = document.getElementById('wizardSourceType');
                const targetType = document.getElementById('wizardTargetType');
                const sourceConnectivity = document.getElementById('wizardSourceConnectivity');
                const targetConnectivity = document.getElementById('wizardTargetConnectivity');
                
                if (!sourceType || !targetType || !sourceConnectivity || !targetConnectivity) {
                    console.log('Some wizard elements not found:', {
                        sourceType: !!sourceType,
                        targetType: !!targetType, 
                        sourceConnectivity: !!sourceConnectivity,
                        targetConnectivity: !!targetConnectivity
                    });
                    return;
                }

                console.log('All connectivity elements found, setting up handlers...');
                
                // Set up event listeners
                this.setupSourceTypeHandler(sourceType, sourceConnectivity);
                this.setupTargetTypeHandler(targetType, targetConnectivity);
                this.setupConnectivityChangeHandlers(sourceConnectivity, targetConnectivity);
                
                this.initialized = true;
                console.log('Connectivity handlers set up successfully for existing form elements');
                
            } catch (error) {
                console.error('Error setting up connectivity handlers:', error);
            }
        }

        /**
         * Set up source type change handler
         */
        setupSourceTypeHandler(sourceTypeElement, sourceConnectivityElement) {
            sourceTypeElement.addEventListener('change', (event) => {
                const sourceType = event.target.value;
                console.log('Source type changed to:', sourceType);
                
                if (sourceType && sourceConnectivityElement) {
                    const defaultConnectivity = this.getDefaultConnectivity('source', sourceType);
                    console.log(`Setting default source connectivity: ${defaultConnectivity}`);
                    sourceConnectivityElement.value = defaultConnectivity;
                    
                    // Trigger change event
                    sourceConnectivityElement.dispatchEvent(new Event('change', { bubbles: true }));
                }
            });
        }

        /**
         * Set up target type change handler
         */
        setupTargetTypeHandler(targetTypeElement, targetConnectivityElement) {
            targetTypeElement.addEventListener('change', (event) => {
                const targetType = event.target.value;
                console.log('Target type changed to:', targetType);
                
                if (targetType && targetConnectivityElement) {
                    const defaultConnectivity = this.getDefaultConnectivity('target', targetType);
                    console.log(`Setting default target connectivity: ${defaultConnectivity}`);
                    targetConnectivityElement.value = defaultConnectivity;
                    
                    // Trigger change event
                    targetConnectivityElement.dispatchEvent(new Event('change', { bubbles: true }));
                }
            });
        }

        /**
         * Set up connectivity change handlers
         */
        setupConnectivityChangeHandlers(sourceConnectivityElement, targetConnectivityElement) {
            sourceConnectivityElement.addEventListener('change', (event) => {
                console.log('Source connectivity changed to:', event.target.value);
                this.handleConnectivityChange('source', event.target.value);
            });
            
            targetConnectivityElement.addEventListener('change', (event) => {
                console.log('Target connectivity changed to:', event.target.value);
                this.handleConnectivityChange('target', event.target.value);
            });
        }

        /**
         * Handle connectivity option changes
         */
        handleConnectivityChange(type, connectivity) {
            // Store in wizard data if available
            if (this.wizard && this.wizard.wizardData) {
                this.wizard.wizardData[`${type}Connectivity`] = connectivity;
                console.log(`Stored ${type}Connectivity in wizard data:`, connectivity);
            }
        }

        /**
         * Get default connectivity for a type
         */
        getDefaultConnectivity(direction, type) {
            const defaults = this.connectivityDefaults[direction];
            return defaults[type] || (direction === 'source' ? 'tcp' : 'http');
        }

        /**
         * Validate connectivity configuration
         */
        validateConnectivity() {
            const sourceType = document.getElementById('wizardSourceType')?.value;
            const targetType = document.getElementById('wizardTargetType')?.value;
            const sourceConnectivity = document.getElementById('wizardSourceConnectivity')?.value;
            const targetConnectivity = document.getElementById('wizardTargetConnectivity')?.value;
            
            const errors = [];
            
            if (sourceType && !sourceConnectivity) {
                errors.push('Source connectivity method is required');
            }
            
            if (targetType && !targetConnectivity) {
                errors.push('Target connectivity method is required');
            }
            
            return {
                isValid: errors.length === 0,
                errors
            };
        }

        /**
         * Get connectivity form data
         */
        getConnectivityData() {
            return {
                sourceConnectivity: document.getElementById('wizardSourceConnectivity')?.value || '',
                targetConnectivity: document.getElementById('wizardTargetConnectivity')?.value || ''
            };
        }

        /**
         * Set connectivity values (useful for loading saved data)
         */
        setConnectivityData(data) {
            if (data.sourceConnectivity) {
                const sourceElement = document.getElementById('wizardSourceConnectivity');
                if (sourceElement) {
                    sourceElement.value = data.sourceConnectivity;
                }
            }
            
            if (data.targetConnectivity) {
                const targetElement = document.getElementById('wizardTargetConnectivity');
                if (targetElement) {
                    targetElement.value = data.targetConnectivity;
                }
            }
        }

        /**
         * Reset connectivity form
         */
        resetConnectivityForm() {
            const sourceConnectivity = document.getElementById('wizardSourceConnectivity');
            const targetConnectivity = document.getElementById('wizardTargetConnectivity');
            
            if (sourceConnectivity) sourceConnectivity.value = '';
            if (targetConnectivity) targetConnectivity.value = '';
        }
    }

    // Initialize connectivity manager
    function initializeConnectivityManager() {
        try {
            // Try to find wizard instance
            const wizard = window.wizard || window.interfaceWizard;
            
            const connectivityManager = new WizardConnectivityManager(wizard);
            connectivityManager.initialize();
            
            // Make it globally available
            window.wizardConnectivityManager = connectivityManager;
            
            // Integrate with wizard reset functionality if it exists
            if (wizard && wizard.resetWizard) {
                const originalReset = wizard.resetWizard.bind(wizard);
                wizard.resetWizard = function() {
                    originalReset();
                    connectivityManager.resetConnectivityForm();
                };
            }
            
            console.log('Wizard connectivity manager initialized');
            
        } catch (error) {
            console.error('Error initializing connectivity manager:', error);
        }
    }

    // Auto-initialize immediately and on DOM ready
    initializeConnectivityManager();
    
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', initializeConnectivityManager);
    }

    // Export for manual access
    window.WizardConnectivityManager = WizardConnectivityManager;
    
    console.log('Wizard connectivity enhancements loaded for existing Step 1 form');

})();

        /**
         * Get connectivity form data
         */
        getConnectivityData() {
            return {
                sourceConnectivity: document.getElementById('wizardSourceConnectivity')?.value || '',
                targetConnectivity: document.getElementById('wizardTargetConnectivity')?.value || ''
            };
        }

        /**
         * Set connectivity values (useful for loading saved data)
         */
        setConnectivityData(data) {
            if (data.sourceConnectivity) {
                const sourceElement = document.getElementById('wizardSourceConnectivity');
                if (sourceElement) {
                    sourceElement.value = data.sourceConnectivity;
                }
            }
            
            if (data.targetConnectivity) {
                const targetElement = document.getElementById('wizardTargetConnectivity');
                if (targetElement) {
                    targetElement.value = data.targetConnectivity;
                }
            }
        }

        /**
         * Reset connectivity form
         */
        resetConnectivityForm() {
            const sourceConnectivity = document.getElementById('wizardSourceConnectivity');
            const targetConnectivity = document.getElementById('wizardTargetConnectivity');
            
            if (sourceConnectivity) sourceConnectivity.value = '';
            if (targetConnectivity) targetConnectivity.value = '';
        }
    }

    // Initialize when wizard is available
    function initializeConnectivityManager() {
        try {
            const wizard = window.wizard || window.interfaceWizard;
            
            if (wizard) {
                const connectivityManager = new WizardConnectivityManager(wizard);
                connectivityManager.initialize();
                
                // Make it globally available
                window.wizardConnectivityManager = connectivityManager;
                
                // Integrate with wizard reset functionality
                if (wizard.resetWizard) {
                    const originalReset = wizard.resetWizard.bind(wizard);
                    wizard.resetWizard = function() {
                        originalReset();
                        connectivityManager.resetConnectivityForm();
                    };
                }
                
                console.log('Wizard connectivity manager initialized successfully');
            } else {
                console.log('Wizard not found, will retry...');
                setTimeout(initializeConnectivityManager, 1000);
            }
            
        } catch (error) {
            console.error('Error initializing connectivity manager:', error);
        }
    }

    // Auto-initialize
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', initializeConnectivityManager);
    } else {
        initializeConnectivityManager();
    }

    // Also initialize when wizard becomes available
    let retryCount = 0;
    const maxRetries = 20;
    
    const retryInit = setInterval(() => {
        retryCount++;
        
        if (window.wizard || window.interfaceWizard) {
            clearInterval(retryInit);
            initializeConnectivityManager();
        } else if (retryCount >= maxRetries) {
            clearInterval(retryInit);
            console.log('Max retries reached, connectivity manager may not be fully initialized');
        }
    }, 500);

    // Export for manual initialization
    window.WizardConnectivityManager = WizardConnectivityManager;
    
    console.log('Wizard connectivity enhancements loaded');

})();