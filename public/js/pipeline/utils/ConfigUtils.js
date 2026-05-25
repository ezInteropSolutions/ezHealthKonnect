/**
 * ConfigUtils - Configuration Object Utilities
 *
 * Purpose: Pure utility functions for configuration object manipulation
 * - Deep merging of configuration objects
 * - Deep cloning (multiple strategies)
 * - Data sanitization
 * - Type coercion
 *
 * Design: Stateless utility class (all static methods)
 *
 * @class ConfigUtils
 */
class ConfigUtils {
    /**
     * Deep merge two configuration objects
     * Later object (source) overwrites earlier object (target)
     *
     * Rules:
     * - Primitives: source overwrites target
     * - Arrays: source replaces target (no merging)
     * - Objects: recursively merge properties
     * - null/undefined: treated as values (can overwrite)
     *
     * @param {Object} target - Base configuration
     * @param {Object} source - Overriding configuration
     * @returns {Object} Merged configuration (new object)
     *
     * @example
     * const base = { a: 1, b: { x: 10 } };
     * const override = { b: { y: 20 }, c: 3 };
     * const result = ConfigUtils.mergeConfig(base, override);
     * // { a: 1, b: { x: 10, y: 20 }, c: 3 }
     */
    static mergeConfig(target, source) {
        // Handle null/undefined
        if (!target) return source;
        if (!source) return target;

        // Deep clone target to avoid mutation
        const result = ConfigUtils.deepClone(target);

        // Handle non-objects
        if (typeof source !== 'object' || Array.isArray(source)) {
            return source;
        }

        // Merge object properties
        for (const key in source) {
            if (!source.hasOwnProperty(key)) continue;

            const sourceValue = source[key];
            const targetValue = result[key];

            // Handle nested objects (but not arrays)
            if (
                sourceValue &&
                typeof sourceValue === 'object' &&
                !Array.isArray(sourceValue) &&
                targetValue &&
                typeof targetValue === 'object' &&
                !Array.isArray(targetValue)
            ) {
                // Recursively merge nested objects
                result[key] = ConfigUtils.mergeConfig(targetValue, sourceValue);
            } else {
                // Direct assignment for primitives, arrays, and null
                result[key] = sourceValue;
            }
        }

        return result;
    }

    /**
     * Deep clone an object
     * Uses multiple strategies with fallbacks
     *
     * Strategy priority:
     * 1. structuredClone() - Modern browsers, best performance
     * 2. JSON.parse(JSON.stringify()) - Fallback, works for most cases
     * 3. Shallow copy - Last resort
     *
     * @param {*} obj - Object to clone
     * @returns {*} Cloned object
     *
     * @example
     * const original = { a: { b: { c: 123 } } };
     * const cloned = ConfigUtils.deepClone(original);
     * cloned.a.b.c = 456;
     * console.log(original.a.b.c); // 123 (unchanged)
     */
    static deepClone(obj) {
        // Handle primitives and null
        if (obj === null || typeof obj !== 'object') {
            return obj;
        }

        // Strategy 1: Use structuredClone if available (modern browsers)
        if (typeof structuredClone === 'function') {
            try {
                return structuredClone(obj);
            } catch (e) {
                console.warn('[ConfigUtils] structuredClone failed, using JSON fallback:', e.message);
            }
        }

        // Strategy 2: JSON clone (works for most cases)
        try {
            return JSON.parse(JSON.stringify(obj));
        } catch (e) {
            console.warn('[ConfigUtils] JSON clone failed, using shallow copy:', e.message);
        }

        // Strategy 3: Shallow copy (last resort)
        if (Array.isArray(obj)) {
            return [...obj];
        }

        return { ...obj };
    }

    /**
     * Sanitize and normalize data based on type
     * Coerces values to expected types, trims strings, etc.
     *
     * @param {*} value - Value to sanitize
     * @param {string} expectedType - Expected type ('string', 'number', 'boolean', etc.)
     * @param {Object} options - Sanitization options
     * @returns {*} Sanitized value
     *
     * @example
     * ConfigUtils.sanitize('  hello  ', 'string', { trim: true }); // 'hello'
     * ConfigUtils.sanitize('123', 'number'); // 123
     * ConfigUtils.sanitize('true', 'boolean'); // true
     */
    static sanitize(value, expectedType, options = {}) {
        if (value === null || value === undefined) {
            return value;
        }

        switch (expectedType) {
            case 'string':
                return ConfigUtils._sanitizeString(value, options);

            case 'number':
                return ConfigUtils._sanitizeNumber(value, options);

            case 'boolean':
                return ConfigUtils._sanitizeBoolean(value, options);

            case 'array':
                return ConfigUtils._sanitizeArray(value, options);

            case 'object':
                return ConfigUtils._sanitizeObject(value, options);

            default:
                return value;
        }
    }

    /**
     * Sanitize string value
     * @private
     */
    static _sanitizeString(value, options) {
        let str = String(value);

        // Trim by default (unless explicitly disabled)
        if (options.trim !== false) {
            str = str.trim();
        }

        // Convert to lowercase if requested
        if (options.lowercase) {
            str = str.toLowerCase();
        }

        // Convert to uppercase if requested
        if (options.uppercase) {
            str = str.toUpperCase();
        }

        return str;
    }

    /**
     * Sanitize number value
     * @private
     */
    static _sanitizeNumber(value, options) {
        if (typeof value === 'number') {
            return value;
        }

        const num = Number(value);

        if (isNaN(num)) {
            return options.defaultValue !== undefined ? options.defaultValue : value;
        }

        // Round to integer if requested
        if (options.integer) {
            return Math.round(num);
        }

        // Round to decimal places if specified
        if (options.decimals !== undefined) {
            return Number(num.toFixed(options.decimals));
        }

        return num;
    }

    /**
     * Sanitize boolean value
     * @private
     */
    static _sanitizeBoolean(value, options) {
        if (typeof value === 'boolean') {
            return value;
        }

        // Truthy values
        if (value === 'true' || value === '1' || value === 1 || value === 'yes') {
            return true;
        }

        // Falsy values
        if (value === 'false' || value === '0' || value === 0 || value === 'no' || value === '') {
            return false;
        }

        return options.defaultValue !== undefined ? options.defaultValue : Boolean(value);
    }

    /**
     * Sanitize array value
     * @private
     */
    static _sanitizeArray(value, options) {
        if (!Array.isArray(value)) {
            // Try to convert to array
            if (typeof value === 'string') {
                // Try JSON parse
                try {
                    const parsed = JSON.parse(value);
                    if (Array.isArray(parsed)) {
                        return parsed;
                    }
                } catch (e) {
                    // Not JSON, split by delimiter
                    const delimiter = options.delimiter || ',';
                    return value.split(delimiter).map(item => item.trim());
                }
            }

            // Wrap in array if not array-like
            return [value];
        }

        // Sanitize each item if type specified
        if (options.itemType) {
            return value.map(item => ConfigUtils.sanitize(item, options.itemType, options.itemOptions || {}));
        }

        return value;
    }

    /**
     * Sanitize object value
     * @private
     */
    static _sanitizeObject(value, options) {
        if (typeof value !== 'object' || value === null || Array.isArray(value)) {
            // Try to parse JSON string
            if (typeof value === 'string') {
                try {
                    return JSON.parse(value);
                } catch (e) {
                    return options.defaultValue !== undefined ? options.defaultValue : value;
                }
            }

            return options.defaultValue !== undefined ? options.defaultValue : value;
        }

        return value;
    }

    /**
     * Check if two objects are deeply equal
     *
     * @param {*} obj1 - First object
     * @param {*} obj2 - Second object
     * @returns {boolean} True if deeply equal
     *
     * @example
     * ConfigUtils.isEqual({ a: 1 }, { a: 1 }); // true
     * ConfigUtils.isEqual({ a: { b: 1 } }, { a: { b: 1 } }); // true
     * ConfigUtils.isEqual({ a: 1 }, { a: 2 }); // false
     */
    static isEqual(obj1, obj2) {
        // Same reference
        if (obj1 === obj2) return true;

        // Different types or one is null
        if (typeof obj1 !== typeof obj2 || obj1 === null || obj2 === null) {
            return false;
        }

        // Primitives
        if (typeof obj1 !== 'object') {
            return obj1 === obj2;
        }

        // Arrays
        if (Array.isArray(obj1) && Array.isArray(obj2)) {
            if (obj1.length !== obj2.length) return false;

            for (let i = 0; i < obj1.length; i++) {
                if (!ConfigUtils.isEqual(obj1[i], obj2[i])) {
                    return false;
                }
            }

            return true;
        }

        // Objects
        const keys1 = Object.keys(obj1);
        const keys2 = Object.keys(obj2);

        if (keys1.length !== keys2.length) return false;

        for (const key of keys1) {
            if (!keys2.includes(key)) return false;
            if (!ConfigUtils.isEqual(obj1[key], obj2[key])) return false;
        }

        return true;
    }

    /**
     * Pick specific properties from object
     *
     * @param {Object} obj - Source object
     * @param {string[]} keys - Keys to pick
     * @returns {Object} New object with only specified keys
     *
     * @example
     * const obj = { a: 1, b: 2, c: 3 };
     * ConfigUtils.pick(obj, ['a', 'c']); // { a: 1, c: 3 }
     */
    static pick(obj, keys) {
        const result = {};

        for (const key of keys) {
            if (obj.hasOwnProperty(key)) {
                result[key] = obj[key];
            }
        }

        return result;
    }

    /**
     * Omit specific properties from object
     *
     * @param {Object} obj - Source object
     * @param {string[]} keys - Keys to omit
     * @returns {Object} New object without specified keys
     *
     * @example
     * const obj = { a: 1, b: 2, c: 3 };
     * ConfigUtils.omit(obj, ['b']); // { a: 1, c: 3 }
     */
    static omit(obj, keys) {
        const result = {};

        for (const key in obj) {
            if (obj.hasOwnProperty(key) && !keys.includes(key)) {
                result[key] = obj[key];
            }
        }

        return result;
    }

    /**
     * Get nested property value using dot notation
     *
     * @param {Object} obj - Source object
     * @param {string} path - Dot-separated path (e.g., 'user.address.city')
     * @param {*} defaultValue - Default value if path doesn't exist
     * @returns {*} Value at path or defaultValue
     *
     * @example
     * const obj = { user: { address: { city: 'NYC' } } };
     * ConfigUtils.get(obj, 'user.address.city'); // 'NYC'
     * ConfigUtils.get(obj, 'user.phone', 'N/A'); // 'N/A'
     */
    static get(obj, path, defaultValue = undefined) {
        if (!obj || typeof obj !== 'object') {
            return defaultValue;
        }

        const keys = path.split('.');
        let current = obj;

        for (const key of keys) {
            if (current === null || current === undefined || typeof current !== 'object') {
                return defaultValue;
            }

            current = current[key];
        }

        return current !== undefined ? current : defaultValue;
    }

    /**
     * Set nested property value using dot notation
     *
     * @param {Object} obj - Target object
     * @param {string} path - Dot-separated path
     * @param {*} value - Value to set
     *
     * @example
     * const obj = {};
     * ConfigUtils.set(obj, 'user.address.city', 'NYC');
     * // obj = { user: { address: { city: 'NYC' } } }
     */
    static set(obj, path, value) {
        if (!obj || typeof obj !== 'object') {
            throw new Error('Target must be an object');
        }

        const keys = path.split('.');
        const lastKey = keys.pop();
        let current = obj;

        // Navigate to parent, creating objects as needed
        for (const key of keys) {
            if (!current[key] || typeof current[key] !== 'object') {
                current[key] = {};
            }
            current = current[key];
        }

        // Set value
        current[lastKey] = value;
    }
}

// ========================================
// EXPORT
// ========================================

if (typeof window !== 'undefined') {
    window.ConfigUtils = ConfigUtils;
}

if (typeof module !== 'undefined' && module.exports) {
    module.exports = ConfigUtils;
}
