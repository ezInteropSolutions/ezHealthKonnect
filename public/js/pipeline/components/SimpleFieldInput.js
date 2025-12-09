/**
 * SimpleFieldInput - Lightweight field path input (NO autocomplete, NO memory leaks)
 * Enterprise-grade: Simple, fast, zero overhead
 */

class SimpleFieldInput {
    constructor(container, options = {}) {
        this.container = container;
        this.options = {
            initialValue: options.initialValue || '',
            onChange: options.onChange || (() => {}),
            searchMode: options.searchMode || 'path'
        };

        this.render();
        this.attachEventListeners();
    }

    render() {
        const placeholder = this.options.searchMode === 'path'
            ? 'e.g., enhancedSegments.PID.fields[2].value'
            : 'e.g., Date of Birth, Patient Name';

        const helpText = this.options.searchMode === 'path'
            ? 'Tip: Use enhancedSegments.{SEGMENT}.fields[{index}].value'
            : 'Tip: Enter the field description you\'re looking for';

        this.container.innerHTML = `
            <div class="simple-field-input-wrapper">
                <input
                    type="text"
                    class="simple-field-input form-control"
                    placeholder="${placeholder}"
                    value="${this.options.initialValue}"
                />
                <button type="button" class="btn btn-sm btn-link help-btn" title="Field Path Help">
                    <i class="fas fa-question-circle"></i>
                </button>
            </div>
            <small class="field-help-text text-muted">${helpText}</small>
        `;

        this.input = this.container.querySelector('.simple-field-input');
        this.helpBtn = this.container.querySelector('.help-btn');
    }

    attachEventListeners() {
        // Simple change handler - no autocomplete overhead
        this.input.addEventListener('change', () => {
            this.options.onChange(this.input.value.trim());
        });

        // Help button - shows examples
        this.helpBtn.addEventListener('click', () => {
            this.showHelp();
        });
    }

    showHelp() {
        const examples = this.options.searchMode === 'path'
            ? `Common Field Paths:
• Patient ID: enhancedSegments.PID.fields[0].value
• Patient Name: enhancedSegments.PID.fields[1].value
• Date of Birth: enhancedSegments.PID.fields[2].value
• Sex: enhancedSegments.PID.fields[3].value
• Message Type: enhancedSegments.MSH.fields[1].value`
            : `Common Field Descriptions:
• Patient identification fields
• Demographics (DOB, sex, address)
• Visit information
• Order details
• Results and observations`;

        alert(examples);
    }

    setSearchMode(mode) {
        this.options.searchMode = mode;
        const placeholder = mode === 'path'
            ? 'e.g., enhancedSegments.PID.fields[2].value'
            : 'e.g., Date of Birth, Patient Name';

        const helpText = mode === 'path'
            ? 'Tip: Use enhancedSegments.{SEGMENT}.fields[{index}].value'
            : 'Tip: Enter the field description you\'re looking for';

        this.input.placeholder = placeholder;
        this.container.querySelector('.field-help-text').textContent = helpText;
    }

    getValue() {
        return this.input.value.trim();
    }

    setValue(value) {
        this.input.value = value;
    }

    destroy() {
        // Simple cleanup - no event listeners to track
        this.input = null;
        this.helpBtn = null;
        this.container = null;
    }
}

// Make available globally
if (typeof window !== 'undefined') {
    window.SimpleFieldInput = SimpleFieldInput;
}
