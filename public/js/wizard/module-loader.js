// module-loader.js - Coordinates loading and initialization of all wizard modules
// Ensures proper dependency order and handles module interdependencies

class WizardModuleLoader {
    constructor() {
        this.loadedModules = new Set();
        this.loadingPromises = new Map();
        this.initializationCallbacks = [];
        this.config = {
            basePath: '/js/wizard/', // For ezHealthKonnect: /js/wizard/
            modules: [
                'wizard-services.js',
                'segment-viewer.js', 
                'step-handlers.js',
                'wizard-main.js'
            ],
            dependencies: {
                'segment-viewer.js': ['wizard-services.js'],
                'step-handlers.js': ['wizard-services.js', 'segment-viewer.js'],
                'wizard-main.js': ['wizard-services.js', 'segment-viewer.js', 'step-handlers.js']
            }
        };
    }

    /**
     * Loads all wizard modules in dependency order
     * @returns {Promise} Promise that resolves when all modules are loaded
     */
    async loadAllModules() {
        console.log('🔄 Loading ezHealthKonnect Interface Wizard modules...');
        
        try {
            // Load modules in dependency order
            for (const module of this.config.modules) {
                await this.loadModule(module);
            }
            
            console.log('✅ All wizard modules loaded successfully');
            
            // Initialize the wizard after all modules are loaded
            this.initializeWizard();
            
            return true;
        } catch (error) {
            console.error('❌ Failed to load wizard modules:', error);
            this.showLoadingError(error);
            return false;
        }
    }

    /**
     * Loads a single module with dependency checking
     * @param {string} moduleName - Name of the module to load
     * @returns {Promise} Promise that resolves when module is loaded
     */
    async loadModule(moduleName) {
        // Check if already loaded
        if (this.loadedModules.has(moduleName)) {
            return;
        }

        // Check if currently loading
        if (this.loadingPromises.has(moduleName)) {
            return this.loadingPromises.get(moduleName);
        }

        console.log(`📦 Loading module: ${moduleName}`);

        // Load dependencies first
        const dependencies = this.config.dependencies[moduleName] || [];
        for (const dependency of dependencies) {
            await this.loadModule(dependency);
        }

        // Load the module
        const loadPromise = this.loadScript(this.config.basePath + moduleName);
        this.loadingPromises.set(moduleName, loadPromise);
        
        try {
            await loadPromise;
            this.loadedModules.add(moduleName);
            console.log(`✅ Module loaded: ${moduleName}`);
        } catch (error) {
            console.error(`❌ Failed to load module ${moduleName}:`, error);
            throw error;
        }
    }

    /**
     * Dynamically loads a JavaScript file
     * @param {string} src - Source URL of the script
     * @returns {Promise} Promise that resolves when script is loaded
     */
    loadScript(src) {
        return new Promise((resolve, reject) => {
            // Check if script already exists
            const existingScript = document.querySelector(`script[src="${src}"]`);
            if (existingScript) {
                resolve();
                return;
            }

            const script = document.createElement('script');
            script.src = src;
            script.type = 'text/javascript';
            
            script.onload = () => {
                console.log(`📜 Script loaded: ${src}`);
                resolve();
            };
            
            script.onerror = (error) => {
                console.error(`📜 Script failed to load: ${src}`, error);
                reject(new Error(`Failed to load script: ${src}`));
            };
            
            document.head.appendChild(script);
        });
    }

    /**
     * Initializes the wizard after all modules are loaded
     */
    initializeWizard() {
        console.log('🧙‍♂️ Initializing ezHealthKonnect Interface Wizard...');
        
        try {
            // Initialize wizard if available
            if (typeof InterfaceWizardModal !== 'undefined') {
                if (!window.wizard) {
                    window.wizard = new InterfaceWizardModal();
                    console.log('✅ Wizard instance created and ready');
                }
            } else {
                throw new Error('InterfaceWizardModal class not found after loading modules');
            }

            // Execute any additional initialization callbacks
            this.initializationCallbacks.forEach(callback => {
                try {
                    callback(window.wizard);
                } catch (error) {
                    console.error('❌ Initialization callback failed:', error);
                }
            });

            // Set up global error handling for wizard operations
            this.setupErrorHandling();

            console.log('🎉 ezHealthKonnect Interface Wizard fully initialized and ready!');

        } catch (error) {
            console.error('❌ Failed to initialize wizard:', error);
            this.showInitializationError(error);
        }
    }

    /**
     * Sets up global error handling for wizard operations
     */
    setupErrorHandling() {
        // Catch unhandled promise rejections related to wizard
        window.addEventListener('unhandledrejection', (event) => {
            if (event.reason && event.reason.message && 
                event.reason.message.includes('wizard')) {
                console.error('🚨 Unhandled wizard promise rejection:', event.reason);
                
                if (window.wizard && window.wizard.notificationService) {
                    window.wizard.notificationService.error(
                        'An unexpected error occurred in the wizard. Please try refreshing the page.',
                        10000
                    );
                }
                
                event.preventDefault();
            }
        });

        // Catch global errors related to wizard
        window.addEventListener('error', (event) => {
            if (event.message && event.message.includes('wizard')) {
                console.error('🚨 Global wizard error:', event.error);
                
                if (window.wizard && window.wizard.notificationService) {
                    window.wizard.notificationService.warning(
                        'A wizard component encountered an error. Some features may not work properly.',
                        8000
                    );
                }
            }
        });
    }

    /**
     * Shows a user-friendly loading error
     * @param {Error} error - The loading error
     */
    showLoadingError(error) {
        const errorMessage = `
            <div style="
                position: fixed; 
                top: 50%; 
                left: 50%; 
                transform: translate(-50%, -50%);
                background: white; 
                padding: 30px; 
                border-radius: 12px; 
                box-shadow: 0 8px 32px rgba(0,0,0,0.1);
                border: 1px solid #fee2e2;
                max-width: 500px;
                z-index: 10000;
                text-align: center;
                font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            ">
                <div style="color: #dc2626; font-size: 48px; margin-bottom: 16px;">⚠️</div>
                <h3 style="color: #dc2626; margin: 0 0 16px 0;">Failed to Load Interface Wizard</h3>
                <p style="color: #6b7280; margin: 0 0 20px 0; line-height: 1.5;">
                    The ezHealthKonnect Interface Wizard could not be loaded. 
                    Please check your internet connection and try refreshing the page.
                </p>
                <details style="text-align: left; margin: 16px 0; color: #6b7280; font-size: 12px;">
                    <summary style="cursor: pointer; font-weight: 600;">Technical Details</summary>
                    <pre style="margin: 8px 0; padding: 8px; background: #f9fafb; border-radius: 4px; overflow: auto;">${error.message}</pre>
                </details>
                <button onclick="window.location.reload()" style="
                    background: #1f2937; 
                    color: white; 
                    border: none; 
                    padding: 12px 24px; 
                    border-radius: 8px; 
                    cursor: pointer;
                    font-weight: 600;
                ">
                    Refresh Page
                </button>
            </div>
        `;
        
        document.body.insertAdjacentHTML('beforeend', errorMessage);
    }

    /**
     * Shows a user-friendly initialization error
     * @param {Error} error - The initialization error
     */
    showInitializationError(error) {
        console.error('🚨 Wizard initialization failed:', error);
        
        // Try to show notification if notification service is available
        if (window.wizard && window.wizard.notificationService) {
            window.wizard.notificationService.error(
                'Wizard initialization failed. Some features may not work properly.',
                10000
            );
        } else {
            // Fallback to simple alert
            setTimeout(() => {
                alert('Interface Wizard initialization failed. Please refresh the page and try again.');
            }, 1000);
        }
    }

    /**
     * Adds a callback to be executed after wizard initialization
     * @param {Function} callback - Callback function to execute
     */
    onInitialized(callback) {
        if (typeof callback === 'function') {
            this.initializationCallbacks.push(callback);
        }
    }

    /**
     * Checks if all modules are loaded
     * @returns {boolean} True if all modules are loaded
     */
    areAllModulesLoaded() {
        return this.config.modules.every(module => this.loadedModules.has(module));
    }

    /**
     * Gets loading status for debugging
     * @returns {Object} Loading status information
     */
    getLoadingStatus() {
        return {
            totalModules: this.config.modules.length,
            loadedModules: Array.from(this.loadedModules),
            pendingModules: this.config.modules.filter(m => !this.loadedModules.has(m)),
            allLoaded: this.areAllModulesLoaded(),
            wizardReady: !!(window.wizard && window.wizard.isModalOpen !== undefined)
        };
    }
}

/**
 * Auto-loading functionality - loads wizard when DOM is ready
 */
class WizardAutoLoader {
    constructor() {
        this.moduleLoader = new WizardModuleLoader();
        this.setupAutoLoading();
    }

    setupAutoLoading() {
        // Check if we should auto-load (look for wizard elements on page)
        const shouldAutoLoad = this.shouldAutoLoadWizard();
        
        if (shouldAutoLoad) {
            console.log('🔍 ezHealthKonnect wizard elements detected, auto-loading...');
            this.startLoading();
        } else {
            console.log('ℹ️ No wizard elements detected. Use window.loadWizard() to load manually.');
            // Make manual loading available
            window.loadWizard = () => this.startLoading();
        }
    }

    shouldAutoLoadWizard() {
        // Check for wizard-related elements or attributes that indicate wizard should be loaded
        const indicators = [
            '#wizardModalOverlay',
            '.interface-wizard',
            '[data-wizard]',
            '.header-btn.primary', // Create interface button
            '#createInterfaceBtn'
        ];

        return indicators.some(selector => document.querySelector(selector));
    }

    async startLoading() {
        try {
            // Show loading indicator if there's a place for it
            this.showLoadingIndicator();
            
            // Load all modules
            const success = await this.moduleLoader.loadAllModules();
            
            if (success) {
                this.hideLoadingIndicator();
                console.log('🎉 ezHealthKonnect Interface Wizard is ready to use!');
                
                // Make module loader available for debugging
                window.wizardLoader = this.moduleLoader;
                
                // Fire a custom event to notify other scripts
                window.dispatchEvent(new CustomEvent('wizardReady', {
                    detail: { wizard: window.wizard, loader: this.moduleLoader }
                }));
            }
            
        } catch (error) {
            this.hideLoadingIndicator();
            console.error('❌ Failed to load ezHealthKonnect wizard:', error);
        }
    }

    showLoadingIndicator() {
        // Look for existing loading areas
        const loadingAreas = [
            document.querySelector('.loading-area'),
            document.querySelector('#loading'),
            document.querySelector('.wizard-loading')
        ].filter(Boolean);

        if (loadingAreas.length > 0) {
            loadingAreas.forEach(area => {
                area.innerHTML = `
                    <div style="display: flex; align-items: center; gap: 8px; padding: 12px; background: #f0f9ff; border: 1px solid #0ea5e9; border-radius: 8px; color: #0369a1;">
                        <div style="width: 16px; height: 16px; border: 2px solid #0ea5e9; border-top-color: transparent; border-radius: 50%; animation: spin 1s linear infinite;"></div>
                        <span>Loading ezHealthKonnect Interface Wizard...</span>
                    </div>
                `;
            });
        }
    }

    hideLoadingIndicator() {
        const loadingAreas = [
            document.querySelector('.loading-area'),
            document.querySelector('#loading'),
            document.querySelector('.wizard-loading')
        ].filter(Boolean);

        loadingAreas.forEach(area => {
            area.innerHTML = '';
        });
    }
}

// Initialize auto-loader when DOM is ready
function initializeWizardLoader() {
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', () => {
            window.wizardAutoLoader = new WizardAutoLoader();
        });
    } else {
        window.wizardAutoLoader = new WizardAutoLoader();
    }
}

// Start the loading process
initializeWizardLoader();

// Export for module systems
if (typeof module !== 'undefined' && module.exports) {
    module.exports = { WizardModuleLoader, WizardAutoLoader };
}

// Add CSS animation for loading spinner
if (!document.querySelector('#wizard-loader-styles')) {
    const style = document.createElement('style');
    style.id = 'wizard-loader-styles';
    style.textContent = `
        @keyframes spin {
            to { transform: rotate(360deg); }
        }
    `;
    document.head.appendChild(style);
}