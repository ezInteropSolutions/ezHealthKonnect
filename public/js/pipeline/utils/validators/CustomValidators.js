/**
 * CustomValidators.js
 *
 * Custom validation functions and conditional validation logic.
 * Supports user-defined validators and complex business rules.
 *
 * Part of the modular validation framework.
 */

class CustomValidators {
    /**
     * Execute a custom validation function
     * @param {*} value - The value to validate
     * @param {Function} validatorFn - Custom validator function(value) => boolean | {valid: boolean, message: string}
     * @param {string} path - Current validation path
     * @param {Object} rules - Validation rules object
     * @param {*} fullData - Complete data object for cross-field validation
     * @returns {Object|null} Error object or null if valid
     */
    static validateCustom(value, validatorFn, path, rules, fullData) {
        if (typeof validatorFn !== 'function') {
            console.warn(`[CustomValidators] Invalid custom validator at ${path}`);
            return null;
        }

        try {
            const result = validatorFn(value, fullData, path);

            // Boolean result
            if (typeof result === 'boolean') {
                if (!result) {
                    return {
                        path,
                        message: rules.errorMessage || `${path} failed custom validation`
                    };
                }
                return null;
            }

            // Object result with valid property
            if (result && typeof result === 'object') {
                if (result.valid === false) {
                    return {
                        path,
                        message: result.message || rules.errorMessage || `${path} failed custom validation`
                    };
                }
                return null;
            }

            // Invalid return type
            console.warn(`[CustomValidators] Custom validator at ${path} returned invalid type:`, typeof result);
            return null;

        } catch (error) {
            console.error(`[CustomValidators] Custom validator at ${path} threw error:`, error);
            return {
                path,
                message: `${path} validation error: ${error.message}`
            };
        }
    }

    /**
     * Execute a conditional validation function
     * Validates value only if condition function returns true
     * @param {*} value - The value to validate
     * @param {Function} conditionFn - Condition function(fullData) => boolean
     * @param {Object} validationRules - Validation rules to apply if condition is true
     * @param {string} path - Current validation path
     * @param {*} fullData - Complete data object
     * @param {Function} validateValueFn - Validation engine's validateValue function
     * @returns {Array<Object>} Array of error objects
     */
    static validateConditional(value, conditionFn, validationRules, path, fullData, validateValueFn) {
        if (typeof conditionFn !== 'function') {
            console.warn(`[CustomValidators] Invalid condition function at ${path}`);
            return [];
        }

        try {
            const shouldValidate = conditionFn(fullData, path);

            if (shouldValidate) {
                // Condition is true, apply the validation rules
                const errors = [];
                validateValueFn(value, validationRules, path, errors);
                return errors;
            }

            // Condition is false, skip validation
            return [];

        } catch (error) {
            console.error(`[CustomValidators] Conditional validation at ${path} threw error:`, error);
            return [{
                path,
                message: `Conditional validation error: ${error.message}`
            }];
        }
    }

    /**
     * Register a named custom validator for reuse
     * @param {string} name - Validator name
     * @param {Function} validatorFn - Validator function
     */
    static register(name, validatorFn) {
        if (!this._registry) {
            this._registry = new Map();
        }

        if (typeof validatorFn !== 'function') {
            throw new Error(`[CustomValidators] Validator function must be a function, got ${typeof validatorFn}`);
        }

        this._registry.set(name, validatorFn);
        console.log(`✅ Registered custom validator: ${name}`);
    }

    /**
     * Get a registered custom validator by name
     * @param {string} name - Validator name
     * @returns {Function|null} Validator function or null
     */
    static get(name) {
        if (!this._registry) {
            return null;
        }
        return this._registry.get(name) || null;
    }

    /**
     * Execute a registered custom validator by name
     * @param {string} name - Validator name
     * @param {*} value - Value to validate
     * @param {string} path - Validation path
     * @param {Object} rules - Validation rules
     * @param {*} fullData - Complete data object
     * @returns {Object|null} Error object or null
     */
    static executeNamed(name, value, path, rules, fullData) {
        const validatorFn = this.get(name);

        if (!validatorFn) {
            console.warn(`[CustomValidators] No validator registered with name: ${name}`);
            return {
                path,
                message: `Unknown validator: ${name}`
            };
        }

        return this.validateCustom(value, validatorFn, path, rules, fullData);
    }

    /**
     * Clear all registered custom validators
     * Useful for testing
     */
    static clearRegistry() {
        if (this._registry) {
            this._registry.clear();
        }
    }

    /**
     * Get all registered validator names
     * @returns {Array<string>} Array of validator names
     */
    static getRegisteredNames() {
        if (!this._registry) {
            return [];
        }
        return Array.from(this._registry.keys());
    }
}

// Pre-register common custom validators

/**
 * Validate email format
 */
CustomValidators.register('email', (value) => {
    const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
    if (!emailRegex.test(value)) {
        return {
            valid: false,
            message: 'Invalid email format'
        };
    }
    return { valid: true };
});

/**
 * Validate URL format
 */
CustomValidators.register('url', (value) => {
    try {
        new URL(value);
        return { valid: true };
    } catch {
        return {
            valid: false,
            message: 'Invalid URL format'
        };
    }
});

/**
 * Validate non-empty string (trimmed)
 */
CustomValidators.register('nonEmptyString', (value) => {
    if (typeof value !== 'string' || value.trim() === '') {
        return {
            valid: false,
            message: 'Value must be a non-empty string'
        };
    }
    return { valid: true };
});

/**
 * Validate positive number
 */
CustomValidators.register('positiveNumber', (value) => {
    if (typeof value !== 'number' || value <= 0) {
        return {
            valid: false,
            message: 'Value must be a positive number'
        };
    }
    return { valid: true };
});

/**
 * Validate non-negative number
 */
CustomValidators.register('nonNegativeNumber', (value) => {
    if (typeof value !== 'number' || value < 0) {
        return {
            valid: false,
            message: 'Value must be a non-negative number'
        };
    }
    return { valid: true };
});

/**
 * Validate integer
 */
CustomValidators.register('integer', (value) => {
    if (typeof value !== 'number' || !Number.isInteger(value)) {
        return {
            valid: false,
            message: 'Value must be an integer'
        };
    }
    return { valid: true };
});

console.log('[CustomValidators] Loaded with', CustomValidators.getRegisteredNames().length, 'pre-registered validators');
