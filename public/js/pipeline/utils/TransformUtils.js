/**
 * TransformUtils - Shared transformation utilities
 * OOP-based common transform logic used by:
 * - IfThenElseBuilder (transform action)
 * - SwitchCaseBuilder (transform action)
 * - Field Mapping step (inline transforms)
 *
 * This ensures consistent transform behavior across all UI components
 */

class TransformUtils {
    /**
     * Available transform types with metadata
     */
    static TRANSFORMS = {
        uppercase: {
            name: 'UPPERCASE',
            description: 'Convert text to uppercase',
            hasOptions: false,
            icon: 'text-height'
        },
        lowercase: {
            name: 'lowercase',
            description: 'Convert text to lowercase',
            hasOptions: false,
            icon: 'text-height'
        },
        trim: {
            name: 'Trim whitespace',
            description: 'Remove leading and trailing whitespace',
            hasOptions: false,
            icon: 'compress-alt'
        },
        concat: {
            name: 'Concatenate',
            description: 'Append text or another field value',
            hasOptions: true,
            options: ['appendText', 'appendField'],
            icon: 'link'
        },
        replace: {
            name: 'Replace text',
            description: 'Find and replace text',
            hasOptions: true,
            options: ['findText', 'replaceText'],
            icon: 'exchange-alt'
        },
        regex_replace: {
            name: 'Regex replace',
            description: 'Replace using regular expression',
            hasOptions: true,
            options: ['regexPattern', 'replaceText'],
            icon: 'code'
        },
        substring: {
            name: 'Substring',
            description: 'Extract a portion of text',
            hasOptions: true,
            options: ['startIndex', 'length'],
            icon: 'cut'
        },
        split: {
            name: 'Split & select',
            description: 'Split by delimiter and select part',
            hasOptions: true,
            options: ['delimiter', 'selectIndex'],
            icon: 'columns'
        },
        pad_left: {
            name: 'Pad left',
            description: 'Pad on left to reach length',
            hasOptions: true,
            options: ['padLength', 'padChar'],
            icon: 'indent'
        },
        pad_right: {
            name: 'Pad right',
            description: 'Pad on right to reach length',
            hasOptions: true,
            options: ['padLength', 'padChar'],
            icon: 'outdent'
        }
    };

    /**
     * Get all transform types as array for dropdowns
     */
    static getTransformOptions() {
        return Object.entries(this.TRANSFORMS).map(([value, meta]) => ({
            value,
            label: meta.name,
            description: meta.description,
            hasOptions: meta.hasOptions,
            icon: meta.icon
        }));
    }

    /**
     * Apply a transform to a value (client-side preview)
     * @param {string} value - Input value
     * @param {string} transformType - Type of transform
     * @param {Object} options - Transform-specific options
     * @returns {string} Transformed value
     */
    static applyTransform(value, transformType, options = {}) {
        if (value === null || value === undefined) {
            return '';
        }

        const str = String(value);

        switch (transformType) {
            case 'uppercase':
                return str.toUpperCase();

            case 'lowercase':
                return str.toLowerCase();

            case 'trim':
                return str.trim();

            case 'concat':
                if (options.appendText) {
                    return str + options.appendText;
                }
                // If appendField is specified, that needs backend resolution
                return str;

            case 'replace':
                if (options.findText) {
                    return str.split(options.findText).join(options.replaceText || '');
                }
                return str;

            case 'regex_replace':
                if (options.regexPattern) {
                    try {
                        const regex = new RegExp(options.regexPattern, 'g');
                        return str.replace(regex, options.replaceText || '');
                    } catch (e) {
                        console.warn('Invalid regex pattern:', options.regexPattern);
                        return str;
                    }
                }
                return str;

            case 'substring':
                const start = parseInt(options.startIndex, 10) || 0;
                const length = options.length ? parseInt(options.length, 10) : undefined;
                if (length !== undefined) {
                    return str.substring(start, start + length);
                }
                return str.substring(start);

            case 'split':
                if (options.delimiter) {
                    const parts = str.split(options.delimiter);
                    const index = parseInt(options.selectIndex, 10) || 0;
                    return parts[index] || '';
                }
                return str;

            case 'pad_left':
                const leftLength = parseInt(options.padLength, 10) || 10;
                const leftChar = options.padChar || '0';
                return str.padStart(leftLength, leftChar);

            case 'pad_right':
                const rightLength = parseInt(options.padLength, 10) || 10;
                const rightChar = options.padChar || '0';
                return str.padEnd(rightLength, rightChar);

            default:
                return str;
        }
    }

    /**
     * Render transform options HTML for a given transform type
     * @param {string} transformType - Type of transform
     * @param {Object} currentValues - Current option values
     * @param {string} prefix - ID prefix for form elements
     * @returns {string} HTML string
     */
    static renderTransformOptions(transformType, currentValues = {}, prefix = '') {
        const meta = this.TRANSFORMS[transformType];
        if (!meta || !meta.hasOptions) {
            return '';
        }

        let html = '';

        switch (transformType) {
            case 'concat':
                html = `
                    <input type="text" class="transform-option-input"
                           placeholder="Append text (or use field below)"
                           value="${currentValues.appendText || ''}"
                           data-field="appendText" id="${prefix}appendText">
                    <div class="field-search-container-inline" id="${prefix}appendField-container"></div>
                `;
                break;

            case 'replace':
                html = `
                    <input type="text" class="transform-option-input" placeholder="Find"
                           value="${currentValues.findText || ''}"
                           data-field="findText" id="${prefix}findText">
                    <input type="text" class="transform-option-input" placeholder="Replace with"
                           value="${currentValues.replaceText || ''}"
                           data-field="replaceText" id="${prefix}replaceText">
                `;
                break;

            case 'regex_replace':
                html = `
                    <input type="text" class="transform-option-input" placeholder="Regex pattern"
                           value="${currentValues.regexPattern || ''}"
                           data-field="regexPattern" id="${prefix}regexPattern">
                    <input type="text" class="transform-option-input" placeholder="Replace with ($1, $2 for groups)"
                           value="${currentValues.replaceText || ''}"
                           data-field="replaceText" id="${prefix}replaceText">
                `;
                break;

            case 'substring':
                html = `
                    <input type="number" class="transform-option-input" placeholder="Start index"
                           value="${currentValues.startIndex || 0}" style="width: 70px;"
                           data-field="startIndex" id="${prefix}startIndex">
                    <input type="number" class="transform-option-input" placeholder="Length"
                           value="${currentValues.length || ''}" style="width: 70px;"
                           data-field="length" id="${prefix}length">
                `;
                break;

            case 'split':
                html = `
                    <input type="text" class="transform-option-input" placeholder="Delimiter"
                           value="${currentValues.delimiter || ''}" style="width: 60px;"
                           data-field="delimiter" id="${prefix}delimiter">
                    <input type="number" class="transform-option-input" placeholder="Index"
                           value="${currentValues.selectIndex || 0}" style="width: 60px;"
                           data-field="selectIndex" id="${prefix}selectIndex">
                `;
                break;

            case 'pad_left':
            case 'pad_right':
                html = `
                    <input type="number" class="transform-option-input" placeholder="Length"
                           value="${currentValues.padLength || 10}" style="width: 60px;"
                           data-field="padLength" id="${prefix}padLength">
                    <input type="text" class="transform-option-input" placeholder="Char"
                           value="${currentValues.padChar || '0'}" style="width: 50px;" maxlength="1"
                           data-field="padChar" id="${prefix}padChar">
                `;
                break;
        }

        return html;
    }

    /**
     * Convert UI transform config to backend format
     * @param {Object} config - UI configuration
     * @returns {Object} Backend-compatible configuration
     */
    static toBackendFormat(config) {
        const { sourceField, targetField, transformType, ...options } = config;

        const result = {
            action: 'transform_field',
            source: sourceField,
            target: targetField,
            transformType: transformType || 'uppercase'
        };

        // Add transform-specific options
        switch (transformType) {
            case 'concat':
                result.appendText = options.appendText || '';
                result.appendField = options.appendField || '';
                break;
            case 'replace':
                result.findText = options.findText || '';
                result.replaceText = options.replaceText || '';
                break;
            case 'regex_replace':
                result.regexPattern = options.regexPattern || '';
                result.replaceText = options.replaceText || '';
                break;
            case 'substring':
                result.startIndex = parseInt(options.startIndex, 10) || 0;
                result.length = options.length ? parseInt(options.length, 10) : null;
                break;
            case 'split':
                result.delimiter = options.delimiter || '';
                result.selectIndex = parseInt(options.selectIndex, 10) || 0;
                break;
            case 'pad_left':
            case 'pad_right':
                result.padLength = parseInt(options.padLength, 10) || 10;
                result.padChar = options.padChar || '0';
                break;
        }

        return result;
    }

    /**
     * Convert backend transform config to UI format
     * @param {Object} backendConfig - Backend configuration
     * @returns {Object} UI-compatible configuration
     */
    static fromBackendFormat(backendConfig) {
        return {
            action: 'transform',
            sourceField: backendConfig.source || '',
            targetField: backendConfig.target || '',
            transformType: backendConfig.transformType || 'uppercase',
            appendText: backendConfig.appendText || '',
            appendField: backendConfig.appendField || '',
            findText: backendConfig.findText || '',
            replaceText: backendConfig.replaceText || '',
            regexPattern: backendConfig.regexPattern || '',
            startIndex: backendConfig.startIndex || 0,
            length: backendConfig.length || null,
            delimiter: backendConfig.delimiter || '',
            selectIndex: backendConfig.selectIndex || 0,
            padLength: backendConfig.padLength || 10,
            padChar: backendConfig.padChar || '0'
        };
    }
}

// Export for browser
if (typeof window !== 'undefined') {
    window.TransformUtils = TransformUtils;
}

// Export for Node.js
if (typeof module !== 'undefined' && module.exports) {
    module.exports = TransformUtils;
}
