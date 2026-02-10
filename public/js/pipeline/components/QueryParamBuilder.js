// QueryParamBuilder.js - Visual query parameter builder component
// Provides Postman-like query parameter management for API enrichment steps

class QueryParamBuilder {
    constructor(container, initialParams = {}) {
        this.container = container;
        this.params = initialParams;
        this.rows = [];
        this.init();
    }

    init() {
        this.container.innerHTML = '';
        this.container.className = 'query-param-builder';

        // Create header
        const header = document.createElement('div');
        header.className = 'query-param-builder-header';
        header.innerHTML = `
            <h4>Query Parameters</h4>
            <button class="btn btn-sm btn-primary add-param-btn" type="button">
                <i class="fas fa-plus"></i> Add Parameter
            </button>
        `;
        this.container.appendChild(header);

        // Create URL preview
        const urlPreview = document.createElement('div');
        urlPreview.className = 'url-preview';
        urlPreview.innerHTML = `
            <div class="url-preview-label">URL Preview:</div>
            <div class="url-preview-value">
                <code class="url-preview-text">?</code>
                <button class="btn btn-sm btn-outline-secondary copy-url-btn" type="button" title="Copy URL">
                    <i class="fas fa-copy"></i>
                </button>
            </div>
        `;
        this.container.appendChild(urlPreview);

        this.urlPreviewText = urlPreview.querySelector('.url-preview-text');
        this.copyUrlBtn = urlPreview.querySelector('.copy-url-btn');

        // Create table
        const table = document.createElement('div');
        table.className = 'query-param-builder-table';
        table.innerHTML = `
            <div class="query-param-row query-param-row-labels">
                <div class="query-param-col-checkbox"></div>
                <div class="query-param-col-key">Key</div>
                <div class="query-param-col-value">Value</div>
                <div class="query-param-col-description">Description</div>
                <div class="query-param-col-actions"></div>
            </div>
        `;
        this.container.appendChild(table);

        this.tableBody = document.createElement('div');
        this.tableBody.className = 'query-param-builder-body';
        table.appendChild(this.tableBody);

        // Bulk actions
        const bulkActions = document.createElement('div');
        bulkActions.className = 'bulk-actions';
        bulkActions.innerHTML = `
            <button class="btn btn-sm btn-outline-secondary bulk-enable-btn" type="button">
                <i class="fas fa-check-square"></i> Enable All
            </button>
            <button class="btn btn-sm btn-outline-secondary bulk-disable-btn" type="button">
                <i class="fas fa-square"></i> Disable All
            </button>
            <button class="btn btn-sm btn-outline-danger bulk-delete-btn" type="button">
                <i class="fas fa-trash"></i> Delete All
            </button>
        `;
        this.container.appendChild(bulkActions);

        // Add initial params
        Object.entries(this.params).forEach(([key, value]) => {
            this.addParamRow(key, value, '', true);
        });

        // Add empty row if no params (disabled by default)
        if (this.rows.length === 0) {
            this.addParamRow('', '', '', false);
        }

        // Event listeners
        header.querySelector('.add-param-btn').addEventListener('click', () => {
            this.addParamRow('', '', '', true);
        });

        this.copyUrlBtn.addEventListener('click', () => {
            this.copyUrlToClipboard();
        });

        bulkActions.querySelector('.bulk-enable-btn').addEventListener('click', () => {
            this.bulkEnable(true);
        });

        bulkActions.querySelector('.bulk-disable-btn').addEventListener('click', () => {
            this.bulkEnable(false);
        });

        bulkActions.querySelector('.bulk-delete-btn').addEventListener('click', () => {
            this.bulkDelete();
        });

        // Add styles
        this.injectStyles();

        // Update preview
        this.updateUrlPreview();
    }

    addParamRow(key = '', value = '', description = '', enabled = true) {
        const row = document.createElement('div');
        row.className = 'query-param-row';
        row.dataset.enabled = enabled;

        row.innerHTML = `
            <div class="query-param-col-checkbox">
                <input type="checkbox" class="param-enabled" ${enabled ? 'checked' : ''}>
            </div>
            <div class="query-param-col-key">
                <input type="text" class="form-control form-control-sm param-key"
                       value="${this.escapeHtml(key)}"
                       placeholder="Parameter name">
            </div>
            <div class="query-param-col-value">
                <div class="param-value-search-wrapper"></div>
            </div>
            <div class="query-param-col-description">
                <input type="text" class="form-control form-control-sm param-description"
                       value="${this.escapeHtml(description)}"
                       placeholder="Optional description">
            </div>
            <div class="query-param-col-actions">
                <button class="btn btn-sm btn-danger delete-param-btn" type="button" title="Delete parameter">
                    <i class="fas fa-trash"></i>
                </button>
            </div>
        `;

        this.tableBody.appendChild(row);
        this.rows.push(row);

        // Event listeners
        const keyInput = row.querySelector('.param-key');
        const valueWrapper = row.querySelector('.param-value-search-wrapper');
        const descriptionInput = row.querySelector('.param-description');
        const enabledCheckbox = row.querySelector('.param-enabled');
        const deleteBtn = row.querySelector('.delete-param-btn');

        // Create value input with FieldPathSearchComponent
        const valueInput = document.createElement('input');
        valueInput.type = 'text';
        valueInput.className = 'form-control form-control-sm param-value';
        valueInput.value = this.escapeHtml(value);
        valueInput.placeholder = 'e.g., PID.3 or search...';
        valueWrapper.appendChild(valueInput);

        // Initialize FieldPathSearchComponent if available
        let fieldPathSearch = null;
        if (typeof FieldPathSearchComponent !== 'undefined') {
            fieldPathSearch = new FieldPathSearchComponent(valueInput, {
                onSelect: (fieldPath) => {
                    valueInput.value = fieldPath;
                    this.onChange();
                    this.updateUrlPreview();
                },
                placeholder: 'Search HL7 fields or enter custom value...',
                allowCustom: true,
                showCategories: true
            });
            row._fieldPathSearch = fieldPathSearch;
        }

        keyInput.addEventListener('input', () => {
            // Auto-enable checkbox when user types a key
            if (keyInput.value.trim() && !enabledCheckbox.checked) {
                enabledCheckbox.checked = true;
                row.dataset.enabled = 'true';
            }
            this.onChange();
            this.updateUrlPreview();
        });

        valueInput.addEventListener('input', () => {
            this.onChange();
            this.updateUrlPreview();
        });

        descriptionInput.addEventListener('input', () => this.onChange());

        enabledCheckbox.addEventListener('change', () => {
            row.dataset.enabled = enabledCheckbox.checked;
            this.onChange();
            this.updateUrlPreview();
        });

        deleteBtn.addEventListener('click', () => {
            // Cleanup FieldPathSearchComponent if exists
            if (row._fieldPathSearch) {
                row._fieldPathSearch.destroy();
            }

            row.remove();
            this.rows = this.rows.filter(r => r !== row);

            // Add empty row if all deleted (disabled by default)
            if (this.rows.length === 0) {
                this.addParamRow('', '', '', false);
            }

            this.onChange();
            this.updateUrlPreview();
        });

        return row;
    }

    getParams() {
        console.log('[QueryParamBuilder] 🔍 getParams() called, rows count:', this.rows.length);
        const params = {};
        this.rows.forEach((row, index) => {
            const enabled = row.querySelector('.param-enabled').checked;
            const key = row.querySelector('.param-key').value.trim();
            const value = row.querySelector('.param-value').value.trim();

            console.log(`[QueryParamBuilder] Row #${index}: enabled=${enabled}, key="${key}", value="${value}"`);

            if (enabled && key) {
                params[key] = value;
            }
        });
        console.log('[QueryParamBuilder] ✅ Returning params:', params);
        return params;
    }

    setParams(params) {
        this.params = params;
        this.tableBody.innerHTML = '';
        this.rows = [];

        Object.entries(params).forEach(([key, value]) => {
            this.addParamRow(key, value, '', true);
        });

        if (this.rows.length === 0) {
            this.addParamRow('', '', '', true);
        }

        this.updateUrlPreview();
    }

    updateUrlPreview() {
        const params = this.getParams();
        const queryString = new URLSearchParams(params).toString();
        this.urlPreviewText.textContent = queryString ? `?${queryString}` : '?';
    }

    copyUrlToClipboard() {
        const queryString = this.urlPreviewText.textContent;
        navigator.clipboard.writeText(queryString).then(() => {
            const originalHTML = this.copyUrlBtn.innerHTML;
            this.copyUrlBtn.innerHTML = '<i class="fas fa-check"></i>';
            this.copyUrlBtn.classList.add('btn-success');
            this.copyUrlBtn.classList.remove('btn-outline-secondary');

            setTimeout(() => {
                this.copyUrlBtn.innerHTML = originalHTML;
                this.copyUrlBtn.classList.remove('btn-success');
                this.copyUrlBtn.classList.add('btn-outline-secondary');
            }, 2000);
        });
    }

    bulkEnable(enabled) {
        this.rows.forEach(row => {
            const checkbox = row.querySelector('.param-enabled');
            checkbox.checked = enabled;
            row.dataset.enabled = enabled;
        });
        this.onChange();
        this.updateUrlPreview();
    }

    async bulkDelete() {
        const confirmed = await this.showConfirmDialog(
            'Are you sure you want to delete all query parameters?',
            {
                title: 'Delete All Parameters',
                confirmText: 'Delete All',
                cancelText: 'Cancel',
                type: 'danger'
            }
        );

        if (!confirmed) {
            return;
        }

        this.tableBody.innerHTML = '';
        this.rows = [];
        this.addParamRow('', '', '', true);
        this.onChange();
        this.updateUrlPreview();
    }

    /**
     * Show in-app confirm dialog
     */
    showConfirmDialog(message, options = {}) {
        // Simple Promise-based confirm dialog
        return new Promise((resolve) => {
            const {
                title = 'Confirm',
                confirmText = 'Yes',
                cancelText = 'Cancel',
                type = 'warning'
            } = options;

            const colors = {
                warning: { primary: '#f59e0b', bg: '#fef3c7', border: '#fcd34d' },
                danger: { primary: '#ef4444', bg: '#fee2e2', border: '#fca5a5' },
                info: { primary: '#3b82f6', bg: '#dbeafe', border: '#93c5fd' }
            };
            const color = colors[type] || colors.warning;

            const overlay = document.createElement('div');
            overlay.style.cssText = `
                position: fixed; top: 0; left: 0; right: 0; bottom: 0;
                background: rgba(0, 0, 0, 0.5);
                display: flex; align-items: center; justify-content: center;
                z-index: 100000;
            `;

            overlay.innerHTML = `
                <div style="background: white; border-radius: 12px; max-width: 400px; width: 90%; overflow: hidden; box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.1);">
                    <div style="padding: 16px 20px; border-bottom: 1px solid ${color.border}; background: ${color.bg}; display: flex; align-items: center; gap: 10px;">
                        <span style="font-size: 20px;">${type === 'danger' ? '⚠️' : '❓'}</span>
                        <span style="font-weight: 600; color: #1e293b;">${this.escapeHtml(title)}</span>
                    </div>
                    <div style="padding: 20px; color: #475569; font-size: 14px;">${this.escapeHtml(message)}</div>
                    <div style="padding: 12px 20px; background: #f8fafc; border-top: 1px solid #e2e8f0; display: flex; justify-content: flex-end; gap: 10px;">
                        <button class="cancel-btn" style="padding: 8px 16px; border: 1px solid #e2e8f0; background: white; color: #64748b; border-radius: 6px; cursor: pointer; font-size: 14px;">${this.escapeHtml(cancelText)}</button>
                        <button class="confirm-btn" style="padding: 8px 16px; border: none; background: ${color.primary}; color: white; border-radius: 6px; cursor: pointer; font-size: 14px;">${this.escapeHtml(confirmText)}</button>
                    </div>
                </div>
            `;

            document.body.appendChild(overlay);

            const cleanup = (result) => { overlay.remove(); resolve(result); };

            overlay.querySelector('.confirm-btn').onclick = () => cleanup(true);
            overlay.querySelector('.cancel-btn').onclick = () => cleanup(false);
            overlay.onclick = (e) => { if (e.target === overlay) cleanup(false); };
        });
    }

    onChange() {
        // Trigger custom event
        const event = new CustomEvent('paramsChanged', {
            detail: { params: this.getParams() }
        });
        this.container.dispatchEvent(event);
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    injectStyles() {
        if (document.getElementById('query-param-builder-styles')) return;

        const style = document.createElement('style');
        style.id = 'query-param-builder-styles';
        style.textContent = `
            .query-param-builder {
                border: 1px solid #ddd;
                border-radius: 4px;
                padding: 15px;
                background: #f8f9fa;
            }

            .query-param-builder-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                margin-bottom: 15px;
            }

            .query-param-builder-header h4 {
                margin: 0;
                font-size: 16px;
                font-weight: 600;
            }

            .url-preview {
                background: white;
                border: 1px solid #dee2e6;
                border-radius: 4px;
                padding: 10px;
                margin-bottom: 15px;
            }

            .url-preview-label {
                font-size: 12px;
                font-weight: 600;
                color: #6c757d;
                margin-bottom: 5px;
            }

            .url-preview-value {
                display: flex;
                gap: 10px;
                align-items: center;
            }

            .url-preview-text {
                flex: 1;
                background: #f8f9fa;
                padding: 8px 12px;
                border-radius: 4px;
                font-family: 'Courier New', monospace;
                font-size: 13px;
                word-break: break-all;
            }

            .copy-url-btn {
                flex-shrink: 0;
            }

            .query-param-builder-table {
                background: white;
                border-radius: 4px;
                overflow: hidden;
                margin-bottom: 10px;
            }

            .query-param-row {
                display: grid;
                grid-template-columns: 40px 1fr 1fr 1fr 60px;
                gap: 10px;
                padding: 8px 10px;
                border-bottom: 1px solid #eee;
                align-items: center;
            }

            .query-param-row-labels {
                background: #f1f3f5;
                font-weight: 600;
                font-size: 12px;
                text-transform: uppercase;
                color: #495057;
            }

            .query-param-row[data-enabled="false"] {
                opacity: 0.5;
            }

            .query-param-col-checkbox {
                text-align: center;
            }

            .query-param-col-key input,
            .query-param-col-value input,
            .query-param-col-description input {
                width: 100%;
            }

            .query-param-col-actions {
                text-align: center;
            }

            .delete-param-btn {
                padding: 4px 8px;
            }

            .add-param-btn {
                font-size: 12px;
                padding: 4px 12px;
            }

            .query-param-row:last-child {
                border-bottom: none;
            }

            .query-param-row:hover {
                background: #f8f9fa;
            }

            .bulk-actions {
                display: flex;
                gap: 10px;
                padding: 10px;
                background: white;
                border-radius: 4px;
            }

            .bulk-actions button {
                font-size: 12px;
            }
        `;
        document.head.appendChild(style);
    }
}

// Export for use in other modules
if (typeof module !== 'undefined' && module.exports) {
    module.exports = QueryParamBuilder;
}
