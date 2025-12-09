/**
 * FieldPathSelector - Reusable Field Path Selection Component
 *
 * Auto-detects field path inputs and adds XPath autocomplete
 * Works across all step types (validation, enrichment, mapping, etc.)
 *
 * Usage:
 * 1. Add data attribute to input: data-field-type="xpath"
 * 2. FieldPathSelector.initialize(container, options) - auto-finds and enhances inputs
 *
 * Example:
 * <input type="text"
 *        name="source_field"
 *        data-field-type="xpath"
 *        placeholder="Field path">
 */

class FieldPathSelector {
    /**
     * Initialize all field path selectors in a container
     * @param {HTMLElement} container - Container to search for field path inputs
     * @param {Object} options - Configuration options
     */
    static initialize(container, options = {}) {
        // Default options from pipeline context
        const config = {
            format: options.format || 'hl7v2',
            version: options.version || 'v2.5',
            messageType: options.messageType || '',
            resourceType: options.resourceType || ''
        };

        // Find all inputs with data-field-type="xpath"
        const xpathInputs = container.querySelectorAll('[data-field-type="xpath"]');

        const selectors = [];

        xpathInputs.forEach(input => {
            // Skip if already enhanced
            if (input.dataset.xpathEnhanced === 'true') {
                return;
            }

            // Create wrapper for autocomplete
            const wrapper = document.createElement('div');
            wrapper.className = 'field-path-selector-wrapper';

            // Insert wrapper before input
            input.parentNode.insertBefore(wrapper, input);

            // Move input into wrapper (will be hidden)
            wrapper.appendChild(input);

            // Hide original input
            input.style.display = 'none';

            // Create autocomplete container
            const autocompleteContainer = document.createElement('div');
            autocompleteContainer.className = 'xpath-autocomplete-container';
            wrapper.insertBefore(autocompleteContainer, input);

            // Get initial value from input
            const initialValue = input.value || input.dataset.initialValue || '';

            // Create XPath autocomplete
            if (window.XPathAutocomplete) {
                const autocomplete = new XPathAutocomplete(autocompleteContainer, {
                    format: config.format,
                    version: config.version,
                    messageType: config.messageType,
                    resourceType: config.resourceType,
                    initialValue: initialValue,
                    placeholder: input.placeholder || 'Type to search field paths...',
                    onChange: (path) => {
                        // Update original input (for form submission)
                        input.value = path;

                        // Trigger change event for validation
                        input.dispatchEvent(new Event('change', { bubbles: true }));
                    }
                });

                // Store reference
                input.dataset.xpathEnhanced = 'true';
                input._xpathAutocomplete = autocomplete;

                selectors.push({
                    input: input,
                    autocomplete: autocomplete,
                    wrapper: wrapper
                });
            } else {
                console.warn('XPathAutocomplete component not loaded');
            }
        });

        return selectors;
    }

    /**
     * Update schema configuration for all selectors in container
     * @param {HTMLElement} container - Container with field path selectors
     * @param {Object} options - New schema options
     */
    static updateSchemaOptions(container, options) {
        const enhancedInputs = container.querySelectorAll('[data-xpath-enhanced="true"]');

        enhancedInputs.forEach(input => {
            if (input._xpathAutocomplete) {
                input._xpathAutocomplete.updateOptions({
                    format: options.format,
                    version: options.version,
                    messageType: options.messageType,
                    resourceType: options.resourceType
                });
            }
        });
    }

    /**
     * Get value from field path selector
     * @param {HTMLElement} input - Original input element
     * @returns {string} Selected field path
     */
    static getValue(input) {
        return input.value;
    }

    /**
     * Set value of field path selector
     * @param {HTMLElement} input - Original input element
     * @param {string} value - Field path to set
     */
    static setValue(input, value) {
        input.value = value;

        if (input._xpathAutocomplete) {
            input._xpathAutocomplete.setValue(value);
        }
    }

    /**
     * Clear field path selector
     * @param {HTMLElement} input - Original input element
     */
    static clear(input) {
        input.value = '';

        if (input._xpathAutocomplete) {
            input._xpathAutocomplete.clear();
        }
    }

    /**
     * Destroy field path selector and restore original input
     * @param {HTMLElement} input - Original input element
     */
    static destroy(input) {
        if (input._xpathAutocomplete) {
            input._xpathAutocomplete.destroy();
            delete input._xpathAutocomplete;
        }

        const wrapper = input.closest('.field-path-selector-wrapper');
        if (wrapper && wrapper.parentNode) {
            // Restore original input
            input.style.display = '';
            wrapper.parentNode.insertBefore(input, wrapper);
            wrapper.remove();
        }

        delete input.dataset.xpathEnhanced;
    }

    /**
     * Create field path input programmatically
     * @param {Object} config - Field configuration
     * @returns {HTMLElement} Field path input element
     */
    static createInput(config = {}) {
        const input = document.createElement('input');
        input.type = 'text';
        input.name = config.name || 'field_path';
        input.placeholder = config.placeholder || 'Type to search field paths...';
        input.value = config.value || '';
        input.className = config.className || 'form-control';
        input.setAttribute('data-field-type', 'xpath');

        if (config.required) {
            input.required = true;
        }

        return input;
    }
}

// Export for use in other modules
if (typeof window !== 'undefined') {
    window.FieldPathSelector = FieldPathSelector;
}

if (typeof module !== 'undefined' && module.exports) {
    module.exports = FieldPathSelector;
}
