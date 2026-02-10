/**
 * ValidationEngine - Enterprise-Grade Validation Framework
 *
 * Features:
 * - Schema-based validation with declarative rules
 * - Support for nested objects and arrays
 * - Custom validation functions
 * - Conditional validation (dependent fields)
 * - User-friendly error messages with field paths
 * - Type coercion and normalization
 *
 * Dependencies:
 * - TypeValidators: Type-specific validation rules
 * - ConstraintValidators: Constraint-based validation rules
 * - CustomValidators: Custom validation functions
 * - SchemaBuilder: Fluent API for building schemas
 *
 * @class ValidationEngine
 */
class ValidationEngine {
    constructor() {
        this.errors = [];
    }

    /**
     * Validate data against schema
     *
     * @param {*} data - Data to validate
     * @param {Object} schema - Validation schema
     * @param {string} path - Current field path (for nested validation)
     * @returns {Object} { valid: boolean, errors: ValidationError[] }
     */
    validate(data, schema, path = '') {
        this.errors = [];
        this._validateValue(data, schema, path);

        return {
            valid: this.errors.length === 0,
            errors: this.errors
        };
    }

    /**
     * Internal: Validate a value against schema rules
     * @private
     */
    _validateValue(value, rules, path) {
        // Handle null/undefined
        if (value === null || value === undefined) {
            if (this._isRequired(rules, {})) {
                this._addError(path, rules.errorMessage || `${path || 'Field'} is required`);
            }
            return;
        }

        // Type validation
        if (rules.type) {
            const actualType = TypeValidators.getType(value);

            if (actualType !== rules.type) {
                const message = rules.errorMessage || `${path || 'Field'} must be of type ${rules.type}, got ${actualType}`;
                this._addError(path, message);
                return; // Stop further validation if type is wrong
            }

            // Type-specific validations using TypeValidators
            const typeErrors = TypeValidators[`validate${this._capitalize(rules.type)}`]?.(value, rules, path);
            if (typeErrors && typeErrors.length > 0) {
                typeErrors.forEach(error => this._addError(error.path, error.message));
            }
        }

        // Required validation
        if (this._isRequired(rules, value)) {
            if (TypeValidators.isEmpty(value)) {
                this._addError(path, rules.errorMessage || `${path || 'Field'} is required`);
                return;
            }
        }

        // Enum validation
        if (rules.enum) {
            const error = ConstraintValidators.validateEnum(value, rules.enum, path, rules);
            if (error) {
                this._addError(error.path, error.message);
            }
        }

        // Pattern validation
        if (rules.pattern) {
            const error = ConstraintValidators.validatePattern(value, rules.pattern, path, rules);
            if (error) {
                this._addError(error.path, error.message);
            }
        }

        // Custom validation (inline function)
        if (rules.custom) {
            const error = CustomValidators.validateCustom(value, rules.custom, path, rules, null);
            if (error) {
                this._addError(error.path, error.message);
            }
        }

        // Named custom validator
        if (rules.validator) {
            const error = CustomValidators.executeNamed(rules.validator, value, path, rules, null);
            if (error) {
                this._addError(error.path, error.message);
            }
        }

        // Conditional validation
        if (rules.when) {
            const errors = CustomValidators.validateConditional(
                value,
                rules.when.condition,
                rules.when.rules,
                path,
                null,
                this._validateValue.bind(this)
            );
            if (errors && errors.length > 0) {
                errors.forEach(error => this._addError(error.path, error.message));
            }
        }

        // Dependency validation
        if (rules.dependsOn) {
            const error = ConstraintValidators.validateDependency(value, rules.dependsOn, null, path);
            if (error) {
                this._addError(error.path, error.message);
            }
        }
    }

    /**
     * Check if field is required
     * @private
     */
    _isRequired(rules, value) {
        if (typeof rules.required === 'function') {
            return rules.required(value);
        }
        return rules.required === true;
    }

    /**
     * Capitalize first letter
     * @private
     */
    _capitalize(str) {
        return str.charAt(0).toUpperCase() + str.slice(1);
    }

    /**
     * Add validation error
     * @private
     */
    _addError(path, message) {
        this.errors.push({
            path: path,
            message: message
        });
    }

    /**
     * Get all errors
     */
    getErrors() {
        return this.errors;
    }

    /**
     * Get errors as formatted strings
     */
    getErrorMessages() {
        return this.errors.map(err => {
            if (err.path) {
                return `${err.path}: ${err.message}`;
            }
            return err.message;
        });
    }

    /**
     * Get errors grouped by field path
     */
    getErrorsByPath() {
        const grouped = {};

        this.errors.forEach(err => {
            const path = err.path || 'general';
            if (!grouped[path]) {
                grouped[path] = [];
            }
            grouped[path].push(err.message);
        });

        return grouped;
    }

    /**
     * Clear all errors
     */
    clearErrors() {
        this.errors = [];
    }

    // ========================================
    // UTILITY METHODS
    // ========================================

    /**
     * Sanitize and normalize data based on schema
     * Delegates to ConfigUtils for type coercion
     *
     * @param {*} data - Data to sanitize
     * @param {Object} schema - Validation schema
     * @returns {*} Sanitized data
     */
    static sanitize(data, schema) {
        if (data === null || data === undefined) {
            return data;
        }

        // Use ConfigUtils.sanitize for type coercion
        if (schema.type) {
            const options = {
                trim: schema.trim !== false,
                defaultValue: schema.default
            };
            const sanitized = ConfigUtils.sanitize(data, schema.type, options);

            // Handle complex types
            if (schema.type === 'array' && Array.isArray(sanitized) && schema.items) {
                return sanitized.map(item => ValidationEngine.sanitize(item, schema.items));
            }

            if (schema.type === 'object' && typeof sanitized === 'object' && schema.properties) {
                const sanitizedObj = {};
                for (const [key, propSchema] of Object.entries(schema.properties)) {
                    if (sanitized.hasOwnProperty(key)) {
                        sanitizedObj[key] = ValidationEngine.sanitize(sanitized[key], propSchema);
                    }
                }
                return sanitizedObj;
            }

            return sanitized;
        }

        return data;
    }

    /**
     * Create validation schema builder (fluent API)
     *
     * @returns {SchemaBuilder}
     */
    static schema() {
        return SchemaBuilder.create();
    }
}

// ========================================
// EXPORT
// ========================================

if (typeof window !== 'undefined') {
    window.ValidationEngine = ValidationEngine;
}

if (typeof module !== 'undefined' && module.exports) {
    module.exports = ValidationEngine;
}

console.log('[ValidationEngine] Loaded - enterprise validation framework ready (modular version)');
