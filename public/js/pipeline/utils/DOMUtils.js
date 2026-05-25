/**
 * DOMUtils - DOM Manipulation Utilities
 *
 * Purpose: Pure utility functions for DOM operations
 * - Element creation with attributes
 * - Validation error display
 * - Form element helpers
 * - Common UI patterns
 *
 * Design: Stateless utility class (all static methods)
 *
 * @class DOMUtils
 */
class DOMUtils {
    /**
     * Create a DOM element with attributes and content
     *
     * @param {string} tag - HTML tag name
     * @param {Object} attributes - Attributes to set
     * @param {string|HTMLElement|HTMLElement[]} content - Inner content
     * @returns {HTMLElement} Created element
     *
     * @example
     * const div = DOMUtils.createElement('div', {
     *     class: 'my-class',
     *     id: 'my-id',
     *     'data-value': '123',
     *     style: { color: 'red', fontSize: '14px' }
     * }, 'Hello World');
     */
    static createElement(tag, attributes = {}, content = null) {
        const element = document.createElement(tag);

        // Set attributes
        for (const [key, value] of Object.entries(attributes)) {
            if (key === 'class' || key === 'className') {
                element.className = value;
            } else if (key === 'style' && typeof value === 'object') {
                // Apply style object
                Object.assign(element.style, value);
            } else if (key.startsWith('on') && typeof value === 'function') {
                // Event listener (e.g., onclick)
                const eventName = key.substring(2).toLowerCase();
                element.addEventListener(eventName, value);
            } else {
                // Regular attribute
                element.setAttribute(key, value);
            }
        }

        // Set content
        if (content !== null) {
            if (typeof content === 'string') {
                element.innerHTML = content;
            } else if (content instanceof HTMLElement) {
                element.appendChild(content);
            } else if (Array.isArray(content)) {
                content.forEach(child => {
                    if (child instanceof HTMLElement) {
                        element.appendChild(child);
                    }
                });
            }
        }

        return element;
    }

    /**
     * Clear element content
     *
     * @param {HTMLElement} element - Element to clear
     *
     * @example
     * DOMUtils.clearElement(containerDiv);
     */
    static clearElement(element) {
        if (element) {
            element.innerHTML = '';
        }
    }

    /**
     * Show validation errors in container
     * Creates a styled error box with error list
     *
     * @param {HTMLElement} container - Container element
     * @param {string[]} errors - Array of error messages
     * @param {Object} options - Display options
     *
     * @example
     * DOMUtils.showValidationErrors(container, [
     *     'Field is required',
     *     'Email is invalid'
     * ]);
     */
    static showValidationErrors(container, errors, options = {}) {
        if (!errors || errors.length === 0) {
            return;
        }

        // Remove existing error container
        const existing = container.querySelector('.builder-validation-errors');
        if (existing) {
            existing.remove();
        }

        // Create error container
        const errorContainer = DOMUtils.createElement('div', {
            class: 'builder-validation-errors',
            style: {
                backgroundColor: options.bgColor || '#fee',
                border: `1px solid ${options.borderColor || '#fcc'}`,
                borderRadius: '4px',
                padding: '12px',
                marginBottom: '12px',
                animation: 'fadeIn 0.3s ease'
            }
        });

        // Add title
        const title = DOMUtils.createElement('div', {
            style: {
                fontWeight: 'bold',
                color: options.titleColor || '#c00',
                marginBottom: '8px'
            }
        }, `⚠️ ${options.title || 'Validation Errors'}`);

        errorContainer.appendChild(title);

        // Add error list
        const ul = DOMUtils.createElement('ul', {
            style: {
                margin: '0',
                paddingLeft: '20px',
                color: options.textColor || '#900'
            }
        });

        errors.forEach(error => {
            const li = DOMUtils.createElement('li', {}, error);
            ul.appendChild(li);
        });

        errorContainer.appendChild(ul);

        // Prepend to container (show at top)
        container.insertBefore(errorContainer, container.firstChild);

        // Auto-remove after timeout if specified
        if (options.autoRemove) {
            setTimeout(() => {
                errorContainer.style.animation = 'fadeOut 0.3s ease';
                setTimeout(() => errorContainer.remove(), 300);
            }, options.autoRemove);
        }
    }

    /**
     * Create a form group (label + input/select/textarea)
     *
     * @param {string} label - Label text
     * @param {HTMLElement} inputElement - Input element
     * @param {Object} options - Options { required, helpText, error }
     * @returns {HTMLElement} Form group div
     *
     * @example
     * const input = DOMUtils.createElement('input', { type: 'text', id: 'email' });
     * const formGroup = DOMUtils.createFormGroup('Email Address', input, {
     *     required: true,
     *     helpText: 'Enter your email',
     *     error: 'Email is required'
     * });
     */
    static createFormGroup(label, inputElement, options = {}) {
        const formGroup = DOMUtils.createElement('div', {
            class: 'form-group'
        });

        // Label
        const labelElement = DOMUtils.createElement('label', {
            for: inputElement.id || ''
        }, label);

        if (options.required) {
            const required = DOMUtils.createElement('span', {
                class: 'text-danger',
                style: { marginLeft: '4px' }
            }, '*');
            labelElement.appendChild(required);
        }

        formGroup.appendChild(labelElement);

        // Input
        formGroup.appendChild(inputElement);

        // Help text
        if (options.helpText) {
            const helpText = DOMUtils.createElement('small', {
                class: 'form-text text-muted',
                style: { display: 'block', marginTop: '4px' }
            }, options.helpText);
            formGroup.appendChild(helpText);
        }

        // Error message
        if (options.error) {
            const error = DOMUtils.createElement('small', {
                class: 'form-text text-danger',
                style: { display: 'block', marginTop: '4px' }
            }, options.error);
            formGroup.appendChild(error);
        }

        return formGroup;
    }

    /**
     * Create a button element
     *
     * @param {string} text - Button text
     * @param {string} className - CSS class name
     * @param {Function} onClick - Click handler
     * @param {Object} options - Additional options { icon, disabled, type }
     * @returns {HTMLElement} Button element
     *
     * @example
     * const button = DOMUtils.createButton('Save', 'btn btn-primary', () => {
     *     console.log('Saved!');
     * }, { icon: 'save', type: 'submit' });
     */
    static createButton(text, className = '', onClick = null, options = {}) {
        const button = DOMUtils.createElement('button', {
            type: options.type || 'button',
            class: className,
            disabled: options.disabled || false
        });

        // Add icon if specified
        if (options.icon) {
            const icon = DOMUtils.createElement('i', {
                class: `fas fa-${options.icon}`,
                style: { marginRight: '6px' }
            });
            button.appendChild(icon);
        }

        // Add text
        const textNode = document.createTextNode(text);
        button.appendChild(textNode);

        // Add click handler
        if (onClick) {
            button.addEventListener('click', onClick);
        }

        return button;
    }

    /**
     * Create a select dropdown with options
     *
     * @param {Array<Object>} options - Options array [{ value, label, selected }]
     * @param {Object} attributes - Select attributes
     * @returns {HTMLElement} Select element
     *
     * @example
     * const select = DOMUtils.createSelect([
     *     { value: 'opt1', label: 'Option 1', selected: true },
     *     { value: 'opt2', label: 'Option 2' }
     * ], { id: 'mySelect', class: 'form-control' });
     */
    static createSelect(options, attributes = {}) {
        const select = DOMUtils.createElement('select', attributes);

        options.forEach(opt => {
            const option = DOMUtils.createElement('option', {
                value: opt.value,
                selected: opt.selected || false
            }, opt.label || opt.value);

            select.appendChild(option);
        });

        return select;
    }

    /**
     * Create an input element
     *
     * @param {string} type - Input type
     * @param {Object} attributes - Input attributes
     * @returns {HTMLElement} Input element
     *
     * @example
     * const input = DOMUtils.createInput('text', {
     *     id: 'username',
     *     placeholder: 'Enter username',
     *     value: 'john'
     * });
     */
    static createInput(type, attributes = {}) {
        return DOMUtils.createElement('input', {
            type: type,
            ...attributes
        });
    }

    /**
     * Create a textarea element
     *
     * @param {Object} attributes - Textarea attributes
     * @param {string} value - Initial value
     * @returns {HTMLElement} Textarea element
     *
     * @example
     * const textarea = DOMUtils.createTextarea({
     *     rows: 5,
     *     placeholder: 'Enter text'
     * }, 'Initial value');
     */
    static createTextarea(attributes = {}, value = '') {
        const textarea = DOMUtils.createElement('textarea', attributes);
        textarea.value = value;
        return textarea;
    }

    /**
     * Show/hide element
     *
     * @param {HTMLElement} element - Element to show/hide
     * @param {boolean} visible - True to show, false to hide
     *
     * @example
     * DOMUtils.setVisible(myDiv, false); // Hide
     * DOMUtils.setVisible(myDiv, true);  // Show
     */
    static setVisible(element, visible) {
        if (!element) return;

        if (visible) {
            element.style.display = '';
            element.classList.remove('hidden');
        } else {
            element.style.display = 'none';
            element.classList.add('hidden');
        }
    }

    /**
     * Add CSS class to element
     *
     * @param {HTMLElement} element - Target element
     * @param {string|string[]} classNames - Class name(s) to add
     *
     * @example
     * DOMUtils.addClass(div, 'active');
     * DOMUtils.addClass(div, ['active', 'highlight']);
     */
    static addClass(element, classNames) {
        if (!element) return;

        const classes = Array.isArray(classNames) ? classNames : [classNames];
        element.classList.add(...classes);
    }

    /**
     * Remove CSS class from element
     *
     * @param {HTMLElement} element - Target element
     * @param {string|string[]} classNames - Class name(s) to remove
     *
     * @example
     * DOMUtils.removeClass(div, 'active');
     */
    static removeClass(element, classNames) {
        if (!element) return;

        const classes = Array.isArray(classNames) ? classNames : [classNames];
        element.classList.remove(...classes);
    }

    /**
     * Toggle CSS class on element
     *
     * @param {HTMLElement} element - Target element
     * @param {string} className - Class name to toggle
     * @returns {boolean} True if class is now present
     *
     * @example
     * DOMUtils.toggleClass(div, 'active');
     */
    static toggleClass(element, className) {
        if (!element) return false;
        return element.classList.toggle(className);
    }

    /**
     * Find elements by selector within container
     *
     * @param {string} selector - CSS selector
     * @param {HTMLElement} container - Container element (default: document)
     * @returns {HTMLElement[]} Array of matching elements
     *
     * @example
     * const buttons = DOMUtils.findAll('.btn', myContainer);
     */
    static findAll(selector, container = document) {
        return Array.from(container.querySelectorAll(selector));
    }

    /**
     * Find single element by selector
     *
     * @param {string} selector - CSS selector
     * @param {HTMLElement} container - Container element (default: document)
     * @returns {HTMLElement|null} First matching element or null
     *
     * @example
     * const button = DOMUtils.find('#saveBtn');
     */
    static find(selector, container = document) {
        return container.querySelector(selector);
    }

    /**
     * Remove element from DOM
     *
     * @param {HTMLElement} element - Element to remove
     *
     * @example
     * DOMUtils.remove(myDiv);
     */
    static remove(element) {
        if (element && element.parentNode) {
            element.parentNode.removeChild(element);
        }
    }

    /**
     * Create a card/panel element
     *
     * @param {string} title - Card title
     * @param {HTMLElement|string} content - Card content
     * @param {Object} options - Card options
     * @returns {HTMLElement} Card element
     *
     * @example
     * const card = DOMUtils.createCard('Settings', contentDiv, {
     *     className: 'shadow-sm',
     *     footer: 'Last updated: Today'
     * });
     */
    static createCard(title, content, options = {}) {
        const card = DOMUtils.createElement('div', {
            class: `card ${options.className || ''}`
        });

        // Header
        if (title) {
            const header = DOMUtils.createElement('div', {
                class: 'card-header'
            }, title);
            card.appendChild(header);
        }

        // Body
        const body = DOMUtils.createElement('div', {
            class: 'card-body'
        }, content);
        card.appendChild(body);

        // Footer
        if (options.footer) {
            const footer = DOMUtils.createElement('div', {
                class: 'card-footer'
            }, options.footer);
            card.appendChild(footer);
        }

        return card;
    }
}

// ========================================
// EXPORT
// ========================================

if (typeof window !== 'undefined') {
    window.DOMUtils = DOMUtils;
}

if (typeof module !== 'undefined' && module.exports) {
    module.exports = DOMUtils;
}
