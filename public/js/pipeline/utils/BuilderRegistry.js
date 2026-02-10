/**
 * BuilderRegistry.js
 *
 * Registry for managing step configuration builders.
 * Handles builder registration, lookup, and lifecycle.
 *
 * Part of the modular factory pattern implementation.
 */

class BuilderRegistry {
    constructor() {
        this.builders = new Map();
        this.instances = new WeakMap();
        this.registrationOrder = [];
    }

    /**
     * Register a builder class for a step type
     * @param {string} stepType - Step type identifier
     * @param {Function} builderClass - Builder class constructor
     * @param {Object} options - Registration options
     * @param {boolean} options.singleton - Create singleton instance
     * @param {boolean} options.override - Allow overriding existing registration
     * @returns {BuilderRegistry} This for chaining
     */
    register(stepType, builderClass, options = {}) {
        const {
            singleton = false,
            override = false
        } = options;

        // Validation
        if (!stepType || typeof stepType !== 'string') {
            throw new Error('[BuilderRegistry] Step type must be a non-empty string');
        }

        if (typeof builderClass !== 'function') {
            throw new Error(`[BuilderRegistry] Builder class for "${stepType}" must be a constructor function`);
        }

        // Check for existing registration
        if (this.builders.has(stepType) && !override) {
            console.warn(`[BuilderRegistry] Builder already registered for "${stepType}", use override option to replace`);
            return this;
        }

        // Store registration
        this.builders.set(stepType, {
            builderClass,
            singleton,
            registeredAt: new Date()
        });

        // Track registration order
        if (!this.registrationOrder.includes(stepType)) {
            this.registrationOrder.push(stepType);
        }

        console.log(`✅ Registered builder: ${stepType}${singleton ? ' (singleton)' : ''}`);
        return this;
    }

    /**
     * Unregister a builder
     * @param {string} stepType - Step type identifier
     * @returns {boolean} True if unregistered, false if not found
     */
    unregister(stepType) {
        const result = this.builders.delete(stepType);

        if (result) {
            const index = this.registrationOrder.indexOf(stepType);
            if (index !== -1) {
                this.registrationOrder.splice(index, 1);
            }
            console.log(`🗑️ Unregistered builder: ${stepType}`);
        }

        return result;
    }

    /**
     * Get builder class for a step type
     * @param {string} stepType - Step type identifier
     * @returns {Function|null} Builder class or null if not found
     */
    get(stepType) {
        const registration = this.builders.get(stepType);
        return registration ? registration.builderClass : null;
    }

    /**
     * Get builder registration info
     * @param {string} stepType - Step type identifier
     * @returns {Object|null} Registration info or null
     */
    getRegistration(stepType) {
        return this.builders.get(stepType) || null;
    }

    /**
     * Create a builder instance
     * @param {string} stepType - Step type identifier
     * @param {HTMLElement} container - Container element
     * @param {Object} initialConfig - Initial configuration
     * @returns {Object|null} Builder instance or null if not found
     */
    createInstance(stepType, container, initialConfig = {}) {
        const registration = this.builders.get(stepType);

        if (!registration) {
            console.warn(`[BuilderRegistry] No builder registered for: ${stepType}`);
            return null;
        }

        const { builderClass, singleton } = registration;

        // Singleton pattern
        if (singleton) {
            let instance = this.instances.get(builderClass);

            if (!instance) {
                instance = new builderClass(container, initialConfig);
                this.instances.set(builderClass, instance);
                console.log(`🏗️ Created singleton instance: ${stepType}`);
            } else {
                // Reinitialize with new container/config
                if (instance.setContainer && typeof instance.setContainer === 'function') {
                    instance.setContainer(container);
                }
                if (instance.setConfig && typeof instance.setConfig === 'function') {
                    instance.setConfig(initialConfig);
                }
                console.log(`♻️ Reused singleton instance: ${stepType}`);
            }

            return instance;
        }

        // Regular instance
        const instance = new builderClass(container, initialConfig);
        console.log(`🏗️ Created instance: ${stepType}`);
        return instance;
    }

    /**
     * Check if a builder is registered
     * @param {string} stepType - Step type identifier
     * @returns {boolean} True if registered
     */
    has(stepType) {
        return this.builders.has(stepType);
    }

    /**
     * Get all registered step types
     * @returns {Array<string>} Array of step type identifiers
     */
    getRegisteredTypes() {
        return Array.from(this.builders.keys());
    }

    /**
     * Get registration count
     * @returns {number} Number of registered builders
     */
    getCount() {
        return this.builders.size;
    }

    /**
     * Clear all registrations
     * Useful for testing
     */
    clear() {
        this.builders.clear();
        this.registrationOrder = [];
        console.log('[BuilderRegistry] Cleared all registrations');
    }

    /**
     * Get registration order
     * @returns {Array<string>} Step types in registration order
     */
    getRegistrationOrder() {
        return [...this.registrationOrder];
    }

    /**
     * Validate all registered builders
     * @returns {Object} Validation results
     */
    validateAll() {
        const results = {
            valid: true,
            errors: [],
            warnings: []
        };

        for (const [stepType, registration] of this.builders.entries()) {
            const { builderClass } = registration;

            // Check if class has required methods
            const requiredMethods = ['getDefaultConfig', 'render', 'attachEvents'];
            const prototype = builderClass.prototype;

            for (const method of requiredMethods) {
                if (typeof prototype[method] !== 'function') {
                    results.valid = false;
                    results.errors.push({
                        stepType,
                        message: `Missing required method: ${method}`
                    });
                }
            }

            // Check if class extends base class (if BaseStepConfigBuilder exists)
            if (typeof BaseStepConfigBuilder !== 'undefined') {
                if (!(prototype instanceof BaseStepConfigBuilder)) {
                    results.warnings.push({
                        stepType,
                        message: 'Does not extend BaseStepConfigBuilder'
                    });
                }
            }
        }

        return results;
    }

    /**
     * Get registry statistics
     * @returns {Object} Statistics object
     */
    getStats() {
        const singletonCount = Array.from(this.builders.values())
            .filter(reg => reg.singleton).length;

        return {
            totalRegistered: this.builders.size,
            singletons: singletonCount,
            regular: this.builders.size - singletonCount,
            oldestRegistration: this.registrationOrder[0] || null,
            newestRegistration: this.registrationOrder[this.registrationOrder.length - 1] || null
        };
    }
}

console.log('[BuilderRegistry] Loaded - builder registration system ready');
