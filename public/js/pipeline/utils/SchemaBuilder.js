/**
 * SchemaBuilder.js
 *
 * Fluent API for building validation schemas.
 * Provides chainable methods for defining validation rules.
 *
 * Part of the modular validation framework.
 */

class SchemaBuilder {
    constructor() {
        this.schema = {};
    }

    /**
     * Define a field in the schema
     * @param {string} fieldName - Field name
     * @returns {FieldBuilder} Field builder for chaining
     */
    field(fieldName) {
        if (!this.schema[fieldName]) {
            this.schema[fieldName] = {};
        }
        return new FieldBuilder(this.schema[fieldName], this, fieldName);
    }

    /**
     * Build and return the final schema
     * @returns {Object} Validation schema
     */
    build() {
        return this.schema;
    }

    /**
     * Create a new schema builder
     * @returns {SchemaBuilder} New schema builder instance
     */
    static create() {
        return new SchemaBuilder();
    }
}

/**
 * Field builder for defining validation rules on a specific field
 */
class FieldBuilder {
    constructor(fieldSchema, schemaBuilder, fieldName) {
        this.fieldSchema = fieldSchema;
        this.schemaBuilder = schemaBuilder;
        this.fieldName = fieldName;
    }

    /**
     * Set field type
     * @param {string} type - Type: 'string', 'number', 'boolean', 'array', 'object'
     * @returns {FieldBuilder} This for chaining
     */
    type(type) {
        this.fieldSchema.type = type;
        return this;
    }

    /**
     * Mark field as required
     * @param {boolean|Function} required - True or conditional function
     * @returns {FieldBuilder} This for chaining
     */
    required(required = true) {
        this.fieldSchema.required = required;
        return this;
    }

    /**
     * Set default value
     * @param {*} defaultValue - Default value
     * @returns {FieldBuilder} This for chaining
     */
    default(defaultValue) {
        this.fieldSchema.default = defaultValue;
        return this;
    }

    /**
     * Set minimum length (strings)
     * @param {number} length - Minimum length
     * @returns {FieldBuilder} This for chaining
     */
    minLength(length) {
        this.fieldSchema.minLength = length;
        return this;
    }

    /**
     * Set maximum length (strings)
     * @param {number} length - Maximum length
     * @returns {FieldBuilder} This for chaining
     */
    maxLength(length) {
        this.fieldSchema.maxLength = length;
        return this;
    }

    /**
     * Set minimum value (numbers)
     * @param {number} value - Minimum value
     * @returns {FieldBuilder} This for chaining
     */
    min(value) {
        this.fieldSchema.min = value;
        return this;
    }

    /**
     * Set maximum value (numbers)
     * @param {number} value - Maximum value
     * @returns {FieldBuilder} This for chaining
     */
    max(value) {
        this.fieldSchema.max = value;
        return this;
    }

    /**
     * Set allowed enum values
     * @param {Array} values - Array of allowed values
     * @returns {FieldBuilder} This for chaining
     */
    enum(values) {
        this.fieldSchema.enum = values;
        return this;
    }

    /**
     * Set regex pattern
     * @param {RegExp|string} pattern - Regex pattern
     * @returns {FieldBuilder} This for chaining
     */
    pattern(pattern) {
        this.fieldSchema.pattern = pattern;
        return this;
    }

    /**
     * Add custom validator function
     * @param {Function} validatorFn - Custom validator
     * @returns {FieldBuilder} This for chaining
     */
    custom(validatorFn) {
        this.fieldSchema.custom = validatorFn;
        return this;
    }

    /**
     * Add named custom validator
     * @param {string} validatorName - Name of registered validator
     * @returns {FieldBuilder} This for chaining
     */
    validator(validatorName) {
        this.fieldSchema.validator = validatorName;
        return this;
    }

    /**
     * Set error message
     * @param {string} message - Error message
     * @returns {FieldBuilder} This for chaining
     */
    errorMessage(message) {
        this.fieldSchema.errorMessage = message;
        return this;
    }

    /**
     * Mark as integer (numbers)
     * @param {boolean} isInteger - True if must be integer
     * @returns {FieldBuilder} This for chaining
     */
    integer(isInteger = true) {
        this.fieldSchema.integer = isInteger;
        return this;
    }

    /**
     * Define array item schema
     * @param {Object} itemSchema - Schema for array items
     * @returns {FieldBuilder} This for chaining
     */
    items(itemSchema) {
        this.fieldSchema.items = itemSchema;
        return this;
    }

    /**
     * Set minimum array length
     * @param {number} length - Minimum length
     * @returns {FieldBuilder} This for chaining
     */
    minItems(length) {
        this.fieldSchema.minItems = length;
        return this;
    }

    /**
     * Set maximum array length
     * @param {number} length - Maximum length
     * @returns {FieldBuilder} This for chaining
     */
    maxItems(length) {
        this.fieldSchema.maxItems = length;
        return this;
    }

    /**
     * Require unique array items
     * @param {Function} keyExtractor - Function to extract unique key
     * @returns {FieldBuilder} This for chaining
     */
    uniqueItems(keyExtractor = null) {
        this.fieldSchema.uniqueItems = keyExtractor || true;
        return this;
    }

    /**
     * Define object properties schema
     * @param {Object} propertiesSchema - Schema for object properties
     * @returns {FieldBuilder} This for chaining
     */
    properties(propertiesSchema) {
        this.fieldSchema.properties = propertiesSchema;
        return this;
    }

    /**
     * Require all object properties to be defined in schema
     * @param {boolean} strict - True for strict mode
     * @returns {FieldBuilder} This for chaining
     */
    strict(strict = true) {
        this.fieldSchema.strict = strict;
        return this;
    }

    /**
     * Add conditional validation
     * @param {Function} conditionFn - Condition function
     * @param {Object} validationRules - Rules to apply when condition is true
     * @returns {FieldBuilder} This for chaining
     */
    when(conditionFn, validationRules) {
        this.fieldSchema.when = {
            condition: conditionFn,
            rules: validationRules
        };
        return this;
    }

    /**
     * Add dependency on another field
     * @param {string} fieldName - Dependent field name
     * @param {*} expectedValue - Expected value (optional)
     * @returns {FieldBuilder} This for chaining
     */
    dependsOn(fieldName, expectedValue = undefined) {
        this.fieldSchema.dependsOn = {
            field: fieldName,
            value: expectedValue
        };
        return this;
    }

    /**
     * End field definition and return to schema builder
     * @returns {SchemaBuilder} Schema builder for defining next field
     */
    end() {
        return this.schemaBuilder;
    }

    /**
     * Build and return final schema (convenience method)
     * @returns {Object} Validation schema
     */
    build() {
        return this.schemaBuilder.build();
    }
}

console.log('[SchemaBuilder] Loaded - fluent schema builder ready');
