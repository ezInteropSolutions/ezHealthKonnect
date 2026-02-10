/**
 * TypeValidators - Type-specific Validation Rules
 *
 * Purpose: Validates values based on their JavaScript type
 * - String validation (length, trimming)
 * - Number validation (range, integer check)
 * - Boolean validation
 * - Array validation (length, items)
 * - Object validation (schema, strict mode)
 *
 * Design: Stateless utility class (all static methods)
 *
 * @class TypeValidators
 */
class TypeValidators {
    /**
     * Validate string value
     *
     * @param {string} value - Value to validate
     * @param {Object} rules - Validation rules
     * @param {string} path - Field path for error messages
     * @returns {string[]} Array of error messages (empty if valid)
     */
    static validateString(value, rules, path) {
        const errors = [];

        // Min length
        if (rules.minLength !== undefined) {
            if (value.length < rules.minLength) {
                const message = rules.errorMessage ||
                    `${path || 'Field'} must be at least ${rules.minLength} characters`;
                errors.push({ path, message });
            }
        }

        // Max length
        if (rules.maxLength !== undefined) {
            if (value.length > rules.maxLength) {
                const message = rules.errorMessage ||
                    `${path || 'Field'} must be at most ${rules.maxLength} characters`;
                errors.push({ path, message });
            }
        }

        return errors;
    }

    /**
     * Validate number value
     *
     * @param {number} value - Value to validate
     * @param {Object} rules - Validation rules
     * @param {string} path - Field path
     * @returns {string[]} Array of error messages
     */
    static validateNumber(value, rules, path) {
        const errors = [];

        // Min value
        if (rules.minValue !== undefined) {
            if (value < rules.minValue) {
                const message = rules.errorMessage ||
                    `${path || 'Field'} must be at least ${rules.minValue}`;
                errors.push({ path, message });
            }
        }

        // Max value
        if (rules.maxValue !== undefined) {
            if (value > rules.maxValue) {
                const message = rules.errorMessage ||
                    `${path || 'Field'} must be at most ${rules.maxValue}`;
                errors.push({ path, message });
            }
        }

        // Integer check
        if (rules.integer === true) {
            if (!Number.isInteger(value)) {
                const message = rules.errorMessage ||
                    `${path || 'Field'} must be an integer`;
                errors.push({ path, message });
            }
        }

        return errors;
    }

    /**
     * Validate boolean value
     *
     * @param {boolean} value - Value to validate
     * @param {Object} rules - Validation rules
     * @param {string} path - Field path
     * @returns {string[]} Array of error messages
     */
    static validateBoolean(value, rules, path) {
        // Booleans don't have specific validation rules beyond type checking
        return [];
    }

    /**
     * Validate array value
     *
     * @param {Array} value - Value to validate
     * @param {Object} rules - Validation rules
     * @param {string} path - Field path
     * @param {Function} validateValueFn - Callback to validate nested values
     * @returns {string[]} Array of error messages
     */
    static validateArray(value, rules, path, validateValueFn) {
        const errors = [];

        // Min length
        if (rules.minLength !== undefined) {
            if (value.length < rules.minLength) {
                const message = rules.errorMessage ||
                    `${path || 'Array'} must have at least ${rules.minLength} items`;
                errors.push({ path, message });
            }
        }

        // Max length
        if (rules.maxLength !== undefined) {
            if (value.length > rules.maxLength) {
                const message = rules.errorMessage ||
                    `${path || 'Array'} must have at most ${rules.maxLength} items`;
                errors.push({ path, message });
            }
        }

        // Validate each item (if validateValueFn provided)
        if (rules.items && validateValueFn) {
            value.forEach((item, index) => {
                const itemPath = path ? `${path}[${index}]` : `[${index}]`;
                const itemErrors = validateValueFn(item, rules.items, itemPath);
                errors.push(...itemErrors);
            });
        }

        return errors;
    }

    /**
     * Validate object value
     *
     * @param {Object} value - Value to validate
     * @param {Object} rules - Validation rules
     * @param {string} path - Field path
     * @param {Function} validateValueFn - Callback to validate nested values
     * @returns {string[]} Array of error messages
     */
    static validateObject(value, rules, path, validateValueFn) {
        const errors = [];

        // Validate each property in schema
        if (rules.schema && validateValueFn) {
            for (const [key, propRules] of Object.entries(rules.schema)) {
                const propPath = path ? `${path}.${key}` : key;
                const propValue = value[key];

                const propErrors = validateValueFn(propValue, propRules, propPath);
                errors.push(...propErrors);
            }
        }

        // Check for unknown properties (strict mode)
        if (rules.strict === true && rules.schema) {
            const allowedKeys = Object.keys(rules.schema);
            const actualKeys = Object.keys(value);

            for (const key of actualKeys) {
                if (!allowedKeys.includes(key)) {
                    const message = `${path ? path + '.' : ''}${key} is not allowed`;
                    errors.push({ path, message });
                }
            }
        }

        return errors;
    }

    /**
     * Check if value is empty
     *
     * @param {*} value - Value to check
     * @returns {boolean} True if empty
     */
    static isEmpty(value) {
        if (value === null || value === undefined) return true;
        if (typeof value === 'string') return value.trim() === '';
        if (Array.isArray(value)) return value.length === 0;
        if (typeof value === 'object') return Object.keys(value).length === 0;
        return false;
    }

    /**
     * Get JavaScript type of value
     *
     * @param {*} value - Value to check
     * @returns {string} Type name ('string', 'number', 'boolean', 'array', 'object', 'null')
     */
    static getType(value) {
        if (value === null) return 'null';
        if (Array.isArray(value)) return 'array';
        return typeof value;
    }

    /**
     * Validate type of value
     *
     * @param {*} value - Value to validate
     * @param {string} expectedType - Expected type
     * @param {string} path - Field path
     * @param {Object} rules - Validation rules (for error message)
     * @returns {Object|null} Error object or null if valid
     */
    static validateType(value, expectedType, path, rules) {
        const actualType = TypeValidators.getType(value);

        if (actualType !== expectedType) {
            const message = rules.errorMessage ||
                `${path || 'Field'} must be of type ${expectedType}, got ${actualType}`;
            return { path, message };
        }

        return null;
    }
}

// ========================================
// EXPORT
// ========================================

if (typeof window !== 'undefined') {
    window.TypeValidators = TypeValidators;
}

if (typeof module !== 'undefined' && module.exports) {
    module.exports = TypeValidators;
}
