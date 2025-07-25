// FIXED module-loader.js - Fixes timing issue and wrong selectors

class WizardModuleLoader {
    constructor() {
        this.loadedModules = new Set();
        this.loadingPromises = new Map();
        this.initializationCallbacks = [];
        this.config = {
            basePath: '/js/wizard/',
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

    async loadAllModules() {
        console.log('🔄 Loading ezHealthKonnect Interface Wizard modules...');
        
        try {
            for (const module of this.config.modules) {
                await this.loadModule(module);
            }
            
            console.log('✅ All wizard modules loaded successfully');
            this.initializeWizard();
            return true;
        } catch (error) {
            console.error('❌ Failed to load wizard modules:', error);
            this.showLoadingError(error);
            return false;
        }
    }

    async loadModule(moduleName) {
        if (this.loadedModules.has(moduleName)) {
            return;
        }

        if (this.loadingPromises.has(moduleName)) {
            return this.loadingPromises.get(moduleName);
        }

        console.log(`📦 Loading module: ${moduleName}`);

        const dependencies = this.config.dependencies[moduleName] || [];
        for (const dependency of dependencies) {
            await this.loadModule(dependency);
        }

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

    loadScript(src) {
        return new Promise((resolve, reject) => {
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

    initializeWizard() {
        console.log('🧙‍♂️ Initializing ezHealthKonnect Interface Wizard...');
        
        try {
            if (typeof InterfaceWizardModal !== 'undefined') {
                if (!window.wizard) {
                    window.wizard = new InterfaceWizardModal();
                    console.log('✅ Wizard instance created and ready');
                }
            } else {
                throw new Error('InterfaceWizardModal class not found after loading modules');
            }

            this.initializationCallbacks.forEach(callback => {
                try {
                    callback(window.wizard);
                } catch (error) {
                    console.error('❌ Initialization callback failed:', error);
                }
            });

            this.setupErrorHandling();
            console.log('🎉 ezHealthKonnect Interface Wizard fully initialized and ready!');

        } catch (error) {
            console.error('❌ Failed to initialize wizard:', error);
            this.showInitializationError(error);
        }
    }

    setupErrorHandling() {
        window.addEventListener('unhandledrejection', (event) => {
            if (event.reason && event.reason.message && 
                event.reason.message.includes('wizard')) {
                console.error('🚨 Unhandled wizard promise rejection:', event.reason);
                
                if (window.wizard && window.wizard.notificationService) {
                    window.wizard.notificationService.error(
                        'An unexpected error occurred in the wizard. Please try refreshing the page.'
                    );
                }
            }
        });
    }

    showLoadingError(error) {
        console.error('🚨 Wizard loading error:', error);
    }

    showInitializationError(error) {
        console.error('🚨 Wizard initialization error:', error);
    }
}

/**
 * FIXED: Auto-loading functionality with proper timing
 */
class WizardAutoLoader {
    constructor() {
        this.moduleLoader = new WizardModuleLoader();
        this.setupAutoLoading();
    }

    setupAutoLoading() {
        // FIXED: Use proper timing - wait for DOM and components to load
        this.waitForComponentsAndAutoLoad();
    }

    waitForComponentsAndAutoLoad() {
        console.log('🔍 Waiting for components to load before auto-detecting wizard elements...');
        
        let attempts = 0;
        const maxAttempts = 20; // 4 seconds total
        
        const checkAndLoad = () => {
            attempts++;
            console.log(`🔍 Auto-load attempt ${attempts}/${maxAttempts}`);
            
            const shouldAutoLoad = this.shouldAutoLoadWizard();
            
            if (shouldAutoLoad) {
                console.log('✅ ezHealthKonnect wizard elements detected, auto-loading...');
                this.startLoading();
            } else if (attempts < maxAttempts) {
                console.log(`⏳ Wizard elements not ready, retrying in 200ms...`);
                setTimeout(checkAndLoad, 200);
            } else {
                console.log('ℹ️ No wizard elements detected after waiting. Use window.loadWizard() to load manually.');
                // Make manual loading available
                window.loadWizard = () => this.startLoading();
            }
        };
        
        // Start checking after a short delay to let initial components load
        setTimeout(checkAndLoad, 500);
    }

    shouldAutoLoadWizard() {
        // FIXED: Updated selectors to match actual HTML
        const indicators = [
            '#wizardModalOverlay',           // From wizard-component.js
            '.interface-wizard',
            '[data-wizard]',
            '.create-btn',                   // FIXED: Actual button class from header-component.js
            '#createInterfaceBtn',           // Actual button ID from header-component.js
            '.header-btn.primary'            // Keep for backward compatibility
        ];

        const found = indicators.some(selector => {
            const element = document.querySelector(selector);
            if (element) {
                console.log(`✅ Found wizard indicator: ${selector}`);
                return true;
            }
            return false;
        });

        if (!found) {
            console.log('❌ No wizard indicators found. Checked:', indicators);
        }

        return found;
    }

    async startLoading() {
        try {
            this.showLoadingIndicator();
            
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

// FIXED: Initialization with proper timing
function initializeWizardLoader() {
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', () => {
            // FIXED: Add slight delay to let other components load first
            setTimeout(() => {
                window.wizardAutoLoader = new WizardAutoLoader();
            }, 100);
        });
    } else {
        // DOM already loaded, start immediately but with delay for components
        setTimeout(() => {
            window.wizardAutoLoader = new WizardAutoLoader();
        }, 100);
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

console.log('✅ FIXED Module loader loaded with timing improvements');