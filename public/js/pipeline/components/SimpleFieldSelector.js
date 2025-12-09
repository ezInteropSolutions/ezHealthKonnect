/**
 * SimpleFieldSelector - Lightweight field path input
 * No complex trees, no memory leaks, just simple text input
 *
 * Usage:
 * const selector = new SimpleFieldSelector(container, {
 *     onChange: (path) => { console.log('Selected:', path); }
 * });
 */

class SimpleFieldSelector {
    constructor(container, options = {}) {
        this.container = container;
        this.options = {
            placeholder: options.placeholder || 'Enter field path (e.g., enhancedSegments.PID.fields[1].value)',
            onChange: options.onChange || (() => {}),
            initialValue: options.initialValue || ''
        };

        this.render();
        this.attachEventListeners();
    }

    render() {
        this.container.innerHTML = `
            <div class="simple-field-selector">
                <div class="field-input-group">
                    <input
                        type="text"
                        class="field-path-input form-control"
                        placeholder="${this.options.placeholder}"
                        value="${this.options.initialValue}"
                    />
                    <button type="button" class="btn btn-sm btn-outline-secondary toggle-helper" title="Show common fields">
                        <i class="fas fa-question-circle"></i>
                    </button>
                </div>
                <div class="field-helper" style="display: none;">
                    <div class="helper-header">
                        <strong>Common Field Paths:</strong>
                        <button type="button" class="btn-close-helper" aria-label="Close">×</button>
                    </div>
                    <div class="helper-content">
                        <div class="helper-section">
                            <div class="helper-label">Patient Information (PID):</div>
                            <button type="button" class="helper-option" data-path="enhancedSegments.PID.fields[0].value">
                                Patient ID (PID.3)
                            </button>
                            <button type="button" class="helper-option" data-path="enhancedSegments.PID.fields[1].value">
                                Patient Name (PID.5)
                            </button>
                            <button type="button" class="helper-option" data-path="enhancedSegments.PID.fields[2].value">
                                Date of Birth (PID.7)
                            </button>
                            <button type="button" class="helper-option" data-path="enhancedSegments.PID.fields[3].value">
                                Administrative Sex (PID.8)
                            </button>
                        </div>
                        <div class="helper-section">
                            <div class="helper-label">Message Header (MSH):</div>
                            <button type="button" class="helper-option" data-path="enhancedSegments.MSH.fields[0].value">
                                Sending Application (MSH.3)
                            </button>
                            <button type="button" class="helper-option" data-path="enhancedSegments.MSH.fields[1].value">
                                Message Type (MSH.9)
                            </button>
                            <button type="button" class="helper-option" data-path="enhancedSegments.MSH.fields[2].value">
                                Message Control ID (MSH.10)
                            </button>
                        </div>
                        <div class="helper-section">
                            <div class="helper-label">Visit Information (PV1):</div>
                            <button type="button" class="helper-option" data-path="enhancedSegments.PV1.fields[0].value">
                                Patient Class (PV1.2)
                            </button>
                            <button type="button" class="helper-option" data-path="enhancedSegments.PV1.fields[1].value">
                                Assigned Location (PV1.3)
                            </button>
                        </div>
                    </div>
                </div>
            </div>
        `;

        this.elements = {
            input: this.container.querySelector('.field-path-input'),
            toggleBtn: this.container.querySelector('.toggle-helper'),
            helper: this.container.querySelector('.field-helper'),
            closeBtn: this.container.querySelector('.btn-close-helper'),
            options: this.container.querySelectorAll('.helper-option')
        };
    }

    attachEventListeners() {
        // Input change
        this.elements.input.addEventListener('change', () => {
            const value = this.elements.input.value.trim();
            this.options.onChange(value);
        });

        // Toggle helper
        this.elements.toggleBtn.addEventListener('click', () => {
            this.toggleHelper();
        });

        // Close helper
        this.elements.closeBtn.addEventListener('click', () => {
            this.hideHelper();
        });

        // Helper options
        this.elements.options.forEach(option => {
            option.addEventListener('click', () => {
                const path = option.dataset.path;
                this.selectPath(path);
            });
        });
    }

    toggleHelper() {
        const isVisible = this.elements.helper.style.display !== 'none';
        if (isVisible) {
            this.hideHelper();
        } else {
            this.showHelper();
        }
    }

    showHelper() {
        this.elements.helper.style.display = 'block';
    }

    hideHelper() {
        this.elements.helper.style.display = 'none';
    }

    selectPath(path) {
        this.elements.input.value = path;
        this.options.onChange(path);
        this.hideHelper();
    }

    getValue() {
        return this.elements.input.value.trim();
    }

    setValue(value) {
        this.elements.input.value = value;
    }

    destroy() {
        // Simple cleanup - no event listeners to remove since we're not using document-level listeners
        this.elements = null;
        this.container = null;
        console.log('[SimpleFieldSelector] Instance destroyed');
    }
}
