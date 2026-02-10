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
            ? [
                { path: 'enhancedSegments.PID.fields[0].value', desc: 'Patient ID' },
                { path: 'enhancedSegments.PID.fields[1].value', desc: 'Patient Name' },
                { path: 'enhancedSegments.PID.fields[2].value', desc: 'Date of Birth' },
                { path: 'enhancedSegments.PID.fields[3].value', desc: 'Sex' },
                { path: 'enhancedSegments.MSH.fields[1].value', desc: 'Message Type' }
            ]
            : [
                { desc: 'Patient identification fields' },
                { desc: 'Demographics (DOB, sex, address)' },
                { desc: 'Visit information' },
                { desc: 'Order details' },
                { desc: 'Results and observations' }
            ];

        // Create modal overlay
        const overlay = document.createElement('div');
        overlay.style.cssText = `
            position: fixed; top: 0; left: 0; right: 0; bottom: 0;
            background: rgba(0, 0, 0, 0.5);
            display: flex; align-items: center; justify-content: center;
            z-index: 100000;
        `;

        const isPathMode = this.options.searchMode === 'path';

        overlay.innerHTML = `
            <div style="background: white; border-radius: 12px; max-width: 500px; width: 90%; overflow: hidden; box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.1);">
                <div style="padding: 16px 20px; background: #dbeafe; display: flex; align-items: center; justify-content: space-between;">
                    <span style="font-weight: 600; color: #1e293b;">
                        📚 ${isPathMode ? 'Common Field Paths' : 'Common Field Descriptions'}
                    </span>
                    <button class="close-btn" style="background: none; border: none; font-size: 20px; cursor: pointer; color: #64748b;">&times;</button>
                </div>
                <div style="padding: 16px 20px; max-height: 300px; overflow-y: auto;">
                    ${examples.map(ex => `
                        <div style="padding: 8px 12px; margin-bottom: 8px; background: #f8fafc; border-radius: 6px; border-left: 3px solid #3b82f6;">
                            ${isPathMode ? `
                                <div style="font-weight: 500; color: #1e293b; margin-bottom: 4px;">${ex.desc}</div>
                                <code style="font-size: 12px; color: #3b82f6; background: #e0f2fe; padding: 2px 6px; border-radius: 4px;">${ex.path}</code>
                            ` : `
                                <div style="color: #475569;">• ${ex.desc}</div>
                            `}
                        </div>
                    `).join('')}
                </div>
                <div style="padding: 12px 20px; background: #f8fafc; border-top: 1px solid #e2e8f0; text-align: right;">
                    <button class="got-it-btn" style="padding: 8px 16px; border: none; background: #3b82f6; color: white; border-radius: 6px; cursor: pointer; font-size: 14px;">Got it</button>
                </div>
            </div>
        `;

        document.body.appendChild(overlay);

        const close = () => overlay.remove();
        overlay.querySelector('.close-btn').onclick = close;
        overlay.querySelector('.got-it-btn').onclick = close;
        overlay.onclick = (e) => { if (e.target === overlay) close(); };
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
