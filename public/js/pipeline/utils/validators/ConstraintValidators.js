/**
 * ConstraintValidators - Constraint-based Validation Rules
 *
 * Purpose: Validates values against specific constraints
 * - Enum (allowed values)
 * - Pattern (regex)
 * - Range constraints (already handled by TypeValidators for numbers)
 *
 * Design: Stateless utility class (all static methods)
 *
 * @class ConstraintValidators
 */
class ConstraintValidators {
    /**
     * Validate enum (allowed values)
     *
     * @param {*} value - Value to validate
     * @param {Array} allowedValues - Array of allowed values
     * @param {string} path - Field path
     * @param {Object} rules - Validation rules (for error message)
     * @returns {Object|null} Error object or null if valid
     */
    static validateEnum(value, allowedValues, path, rules) {
        if (!allowedValues.includes(value)) {
            const allowedStr = allowedValues.map(v => `"${v}"`).join(', ');
            const message = rules.errorMessage ||
                `${path || 'Field'} must be one of: ${allowedStr}`;
            return { path, message };
        }

        return null;
    }

    /**
     * Validate regex pattern
     *
     * @param {*} value - Value to validate
     * @param {string|RegExp} pattern - Regex pattern
     * @param {string} path - Field path
     * @param {Object} rules - Validation rules (for error message)
     * @returns {Object|null} Error object or null if valid
     */
    static validatePattern(value, pattern, path, rules) {
        try {
            const regex = pattern instanceof RegExp ? pattern : new RegExp(pattern);

            if (!regex.test(String(value))) {
                const message = rules.errorMessage ||
                    `${path || 'Field'} does not match required pattern`;
                return { path, message };
            }

            return null;
        } catch (error) {
            const message = `Invalid regex pattern: ${pattern}`;
            return { path, message };
        }
    }

    /**
     * Validate uniqueness in array
     * (Used for checking duplicate keys, etc.)
     *
     * @param {Array} array - Array to check
     * @param {Function} keyExtractor - Function to extract key from item
     * @param {string} path - Field path
     * @returns {Object[]} Array of error objects
     */
    static validateUnique(array, keyExtractor, path) {
        const errors = [];
        const seen = new Map();

        array.forEach((item, index) => {
            const key = keyExtractor(item);

            if (seen.has(key)) {
                const firstIndex = seen.get(key);
                const message = `Duplicate value "${key}" at positions ${firstIndex} and ${index}`;
                errors.push({ path, message });
            } else {
                seen.set(key, index);
            }
        });

        return errors;
    }

    /**
     * Validate dependencies (field required if another field has specific value)
     *
     * @param {*} value - Value to validate
     * @param {Object} dependency - Dependency config { field, value, condition }
     * @param {Object} fullData - Full data object to check dependency
     * @param {string} path - Field path
     * @returns {Object|null} Error object or null if valid
     */
    static validateDependency(value, dependency, fullData, path) {
        // Check if dependency is met
        const depField = dependency.field;
        const depValue = fullData[depField];

        let dependencyMet = false;

        if (dependency.condition === 'equals') {
            dependencyMet = depValue === dependency.value;
        } else if (dependency.condition === 'exists') {
            dependencyMet = depValue !== null && depValue !== undefined;
        } else if (dependency.condition === 'not_equals') {
            dependencyMet = depValue !== dependency.value;
        }

        // If dependency is met, value is required
        if (dependencyMet) {
            if (value === null || value === undefined || value === '') {
                const message = `${path || 'Field'} is required when ${depField} is ${dependency.value}`;
                return { path, message };
            }
        }

        return null;
    }
}

// ========================================
// EXPORT
// ========================================

if (typeof window !== 'undefined') {
    window.ConstraintValidators = ConstraintValidators;
}

if (typeof module !== 'undefined' && module.exports) {
    module.exports = ConstraintValidators;
}
