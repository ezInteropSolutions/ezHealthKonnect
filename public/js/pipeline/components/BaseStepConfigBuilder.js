/**
 * BaseStepConfigBuilder - Abstract Base Class for Step Configuration Builders
 *
 * Implements:
 * - Template Method Pattern: init() defines the algorithm structure
 * - Hook Methods: beforeInit(), afterInit() for extension points
 * - Common Interface: Standardizes API across all builders
 *
 * Design Principles:
 * - Open/Closed: Open for extension (subclass), closed for modification
 * - DRY: Common functionality in one place
 * - Liskov Substitution: All subclasses are interchangeable
 *
 * Dependencies:
 * - ConfigUtils: Configuration management utilities
 * - DOMUtils: DOM manipulation utilities
 *
 * @abstract
 * @class BaseStepConfigBuilder
 */
class BaseStepConfigBuilder {
    /**
     * Constructor
     * @param {HTMLElement} container - DOM container for rendering
     * @param {Object} initialConfig - Initial configuration (optional)
     * @throws {Error} If instantiated directly (must be subclassed)
     */
    constructor(container, initialConfig = {}) {
        // Prevent direct instantiation (abstract class)
        if (new.target === BaseStepConfigBuilder) {
            throw new Error('BaseStepConfigBuilder is abstract and cannot be instantiated directly. Create a subclass instead.');
        }

        // Validate container
        if (!container) {
            throw new Error('Container element is required');
        }

        this.container = container;
        this.initialized = false;
        this.destroyed = false;

        // Get default config from subclass and merge with initial config
        this.config = this.getDefaultConfig();
        if (initialConfig && Object.keys(initialConfig).length > 0) {
            this.config = ConfigUtils.mergeConfig(this.config, initialConfig);
        }

        console.log(`[${this.constructor.name}] Constructed with config:`, this.config);
    }

    // ========================================
    // TEMPLATE METHOD PATTERN
    // ========================================

    /**
     * Main initialization method (Template Method)
     * Defines the algorithm structure - subclasses fill in the details
     *
     * Algorithm:
     * 1. Check if already initialized
     * 2. Call beforeInit() hook
     * 3. Validate prerequisites
     * 4. Render UI (abstract method)
     * 5. Attach event listeners (abstract method)
     * 6. Add custom styles (hook method)
     * 7. Call afterInit() hook
     * 8. Mark as initialized
     *
     * @final - Should not be overridden by subclasses
     */
    init() {
        if (this.initialized) {
            console.warn(`[${this.constructor.name}] Already initialized, skipping...`);
            return;
        }

        if (this.destroyed) {
            throw new Error(`[${this.constructor.name}] Cannot reinitialize a destroyed builder`);
        }

        console.log(`[${this.constructor.name}] 🔄 Initializing...`);

        try {
            // Step 1: Pre-initialization hook
            this.beforeInit();

            // Step 2: Validate prerequisites
            this.validatePrerequisites();

            // Step 3: Render UI (implemented by subclass)
            console.log(`[${this.constructor.name}]   → Rendering UI...`);
            this.render();

            // Step 4: Attach event listeners (implemented by subclass)
            console.log(`[${this.constructor.name}]   → Attaching events...`);
            this.attachEvents();

            // Step 5: Add custom styles (optional hook)
            this.addStyles();

            // Step 6: Post-initialization hook
            this.afterInit();

            this.initialized = true;
            console.log(`[${this.constructor.name}] ✅ Initialized successfully`);

        } catch (error) {
            console.error(`[${this.constructor.name}] ❌ Initialization failed:`, error);
            throw error;
        }
    }

    /**
     * Validates prerequisites before initialization
     * @protected
     */
    validatePrerequisites() {
        if (!this.container) {
            throw new Error('Container element is required');
        }

        if (!(this.container instanceof HTMLElement)) {
            throw new Error('Container must be a valid HTMLElement');
        }
    }

    // ========================================
    // ABSTRACT METHODS (must be implemented by subclasses)
    // ========================================

    /**
     * Returns default configuration structure
     * @abstract
     * @returns {Object} Default configuration object
     * @throws {Error} If not implemented by subclass
     */
    getDefaultConfig() {
        throw new Error(`${this.constructor.name} must implement getDefaultConfig()`);
    }

    /**
     * Renders the UI into the container
     * @abstract
     * @throws {Error} If not implemented by subclass
     */
    render() {
        throw new Error(`${this.constructor.name} must implement render()`);
    }

    /**
     * Attaches event listeners to rendered elements
     * @abstract
     * @throws {Error} If not implemented by subclass
     */
    attachEvents() {
        throw new Error(`${this.constructor.name} must implement attachEvents()`);
    }

    // ========================================
    // HOOK METHODS (optional overrides)
    // ========================================

    /**
     * Hook: Called before initialization begins
     * Override this to add pre-initialization logic
     * @protected
     */
    beforeInit() {
        // Default: no-op
    }

    /**
     * Hook: Called after initialization completes
     * Override this to add post-initialization logic
     * @protected
     */
    afterInit() {
        // Default: no-op
    }

    /**
     * Hook: Adds custom CSS styles
     * Override this to inject builder-specific styles
     * @protected
     */
    addStyles() {
        // Default: no custom styles
    }

    // ========================================
    // PUBLIC API (standardized interface)
    // ========================================

    /**
     * Gets current configuration
     * @returns {Object} Current configuration object
     */
    getConfig() {
        if (this.destroyed) {
            console.warn(`[${this.constructor.name}] Getting config from destroyed builder`);
        }
        return this.config;
    }

    /**
     * Sets configuration and re-renders
     * @param {Object} newConfig - New configuration object
     */
    setConfig(newConfig) {
        if (this.destroyed) {
            throw new Error(`[${this.constructor.name}] Cannot set config on destroyed builder`);
        }

        console.log(`[${this.constructor.name}] Setting new config:`, newConfig);

        // Merge with defaults to ensure all required fields exist
        this.config = ConfigUtils.mergeConfig(this.getDefaultConfig(), newConfig);

        // Re-render if already initialized
        if (this.initialized) {
            console.log(`[${this.constructor.name}] Re-rendering after config change...`);
            this.render();
            this.attachEvents();
        }
    }

    /**
     * Sets container element
     * @param {HTMLElement} container - New container element
     */
    setContainer(container) {
        if (!container || !(container instanceof HTMLElement)) {
            throw new Error('Container must be a valid HTMLElement');
        }
        this.container = container;
        console.log(`[${this.constructor.name}] Container updated`);
    }

    /**
     * Validates current configuration
     * Override this method in subclasses for custom validation
     *
     * @returns {Object} { valid: boolean, errors: string[] }
     */
    validate() {
        // Default: always valid
        // Subclasses should override with specific validation logic
        return {
            valid: true,
            errors: []
        };
    }

    /**
     * Checks if builder is initialized
     * @returns {boolean}
     */
    isInitialized() {
        return this.initialized;
    }

    /**
     * Checks if builder is destroyed
     * @returns {boolean}
     */
    isDestroyed() {
        return this.destroyed;
    }

    // ========================================
    // PROTECTED UTILITY METHODS (delegated to utilities)
    // ========================================

    /**
     * Clears the container
     * @protected
     */
    clear() {
        if (this.container) {
            this.container.innerHTML = '';
        }
    }

    /**
     * Creates a DOM element with attributes
     * @protected
     * @param {string} tag - HTML tag name
     * @param {Object} attributes - Attributes to set
     * @param {string} content - Inner HTML content
     * @returns {HTMLElement}
     */
    createElement(tag, attributes = {}, content = '') {
        return DOMUtils.createElement(tag, attributes, content);
    }

    /**
     * Shows validation errors in the UI
     * @protected
     * @param {string[]} errors - Array of error messages
     */
    showValidationErrors(errors) {
        DOMUtils.showValidationErrors(this.container, errors);
    }

    // ========================================
    // LIFECYCLE MANAGEMENT
    // ========================================

    /**
     * Cleanup and destroy builder
     * Removes DOM elements and releases resources
     *
     * After calling destroy(), the builder cannot be reused
     */
    destroy() {
        if (this.destroyed) {
            console.warn(`[${this.constructor.name}] Already destroyed`);
            return;
        }

        console.log(`[${this.constructor.name}] 🗑️ Destroying...`);

        try {
            // Clear container
            this.clear();

            // Mark as destroyed
            this.destroyed = true;
            this.initialized = false;

            console.log(`[${this.constructor.name}] ✅ Destroyed successfully`);
        } catch (error) {
            console.error(`[${this.constructor.name}] ❌ Destroy failed:`, error);
            throw error;
        }
    }

    // ========================================
    // DEBUGGING & DIAGNOSTICS
    // ========================================

    /**
     * Returns diagnostic information about the builder
     * Useful for debugging
     *
     * @returns {Object} Diagnostic info
     */
    getDiagnostics() {
        return {
            name: this.constructor.name,
            initialized: this.initialized,
            destroyed: this.destroyed,
            hasContainer: !!this.container,
            configKeys: this.config ? Object.keys(this.config) : [],
            configValid: this.validate().valid
        };
    }

    /**
     * Logs diagnostic information to console
     */
    logDiagnostics() {
        console.log(`[${this.constructor.name}] Diagnostics:`, this.getDiagnostics());
    }
}

// ========================================
// EXPORT
// ========================================

if (typeof window !== 'undefined') {
    window.BaseStepConfigBuilder = BaseStepConfigBuilder;
}

if (typeof module !== 'undefined' && module.exports) {
    module.exports = BaseStepConfigBuilder;
}

console.log('[BaseStepConfigBuilder] Loaded - abstract base class ready (modular version)');
