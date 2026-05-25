/**
 * FieldPathInput - Ultra-simple field path input with search toggle
 * NO tree loading, NO API calls, NO memory overhead
 * Just a text input with toggle between "Search by Path" and "Search by Description"
 */

class FieldPathInput {
    constructor(container, options = {}) {
        this.container = container;
        this.options = {
            initialValue: options.initialValue || '',
            onChange: options.onChange || (() => {})
        };

        this.searchMode = 'path'; // 'path' or 'description'
        this.render();
        this.attachEventListeners();
    }

    render() {
        this.container.innerHTML = `
            <div class="field-path-input-wrapper">
                <input
                    type="text"
                    class="field-path-input form-control"
                    placeholder="Enter field path (e.g., enhancedSegments.PID.fields[2].value)"
                    value="${this.options.initialValue}"
                />
                <button type="button" class="btn btn-sm btn-outline-secondary search-toggle" title="Toggle search mode">
                    <i class="fas fa-exchange-alt"></i>
                    <span class="search-mode-text">Path</span>
                </button>
            </div>
            <small class="search-hint">Searching by: <strong class="search-mode-label">Field Path</strong></small>
        `;

        this.elements = {
            input: this.container.querySelector('.field-path-input'),
            toggle: this.container.querySelector('.search-toggle'),
            modeText: this.container.querySelector('.search-mode-text'),
            modeLabel: this.container.querySelector('.search-mode-label')
        };
    }

    attachEventListeners() {
        // Input change
        this.elements.input.addEventListener('change', () => {
            const value = this.elements.input.value.trim();
            this.options.onChange(value);
        });

        // Toggle search mode
        this.elements.toggle.addEventListener('click', () => {
            this.toggleSearchMode();
        });
    }

    toggleSearchMode() {
        this.searchMode = this.searchMode === 'path' ? 'description' : 'path';

        if (this.searchMode === 'path') {
            this.elements.modeText.textContent = 'Path';
            this.elements.modeLabel.textContent = 'Field Path';
            this.elements.input.placeholder = 'Enter field path (e.g., enhancedSegments.PID.fields[2].value)';
        } else {
            this.elements.modeText.textContent = 'Desc';
            this.elements.modeLabel.textContent = 'Description';
            this.elements.input.placeholder = 'Enter description (e.g., Patient Name, Date of Birth)';
        }
    }

    getValue() {
        return this.elements.input.value.trim();
    }

    setValue(value) {
        this.elements.input.value = value;
    }

    destroy() {
        this.elements = null;
        this.container = null;
    }
}

// Make available globally
if (typeof window !== 'undefined') {
    window.FieldPathInput = FieldPathInput;
}
