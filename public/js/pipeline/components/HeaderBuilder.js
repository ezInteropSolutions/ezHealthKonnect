// HeaderBuilder.js - Visual HTTP header builder component
// Provides Postman-like header management for API enrichment steps

class HeaderBuilder {
    constructor(container, initialHeaders = {}) {
        this.container = container;
        this.headers = initialHeaders;
        this.rows = [];
        this.presetHeaders = this.getPresetHeaders();
        this.init();
    }

    init() {
        this.container.innerHTML = '';
        this.container.className = 'header-builder';

        // Create header
        const header = document.createElement('div');
        header.className = 'header-builder-header';
        header.innerHTML = `
            <h4>HTTP Headers</h4>
            <button class="btn btn-sm btn-primary add-header-btn" type="button">
                <i class="fas fa-plus"></i> Add Header
            </button>
        `;
        this.container.appendChild(header);

        // Create table
        const table = document.createElement('div');
        table.className = 'header-builder-table';
        table.innerHTML = `
            <div class="header-row header-row-labels">
                <div class="header-col-checkbox"></div>
                <div class="header-col-key">Key</div>
                <div class="header-col-value">Value</div>
                <div class="header-col-actions"></div>
            </div>
        `;
        this.container.appendChild(table);

        this.tableBody = document.createElement('div');
        this.tableBody.className = 'header-builder-body';
        table.appendChild(this.tableBody);

        // Add initial headers
        Object.entries(this.headers).forEach(([key, value]) => {
            this.addHeaderRow(key, value, true);
        });

        // Add empty row if no headers
        if (this.rows.length === 0) {
            this.addHeaderRow('', '', true);
        }

        // Event listeners
        header.querySelector('.add-header-btn').addEventListener('click', () => {
            this.addHeaderRow('', '', true);
        });

        // Add styles
        this.injectStyles();
    }

    addHeaderRow(key = '', value = '', enabled = true) {
        const row = document.createElement('div');
        row.className = 'header-row';
        row.dataset.enabled = enabled;

        row.innerHTML = `
            <div class="header-col-checkbox">
                <input type="checkbox" class="header-enabled" ${enabled ? 'checked' : ''}>
            </div>
            <div class="header-col-key">
                <input type="text" class="form-control form-control-sm header-key"
                       value="${this.escapeHtml(key)}"
                       placeholder="Header name"
                       list="preset-headers-${this.rows.length}">
                <datalist id="preset-headers-${this.rows.length}">
                    ${this.presetHeaders.map(h => `<option value="${h.key}">${h.description}</option>`).join('')}
                </datalist>
            </div>
            <div class="header-col-value">
                <input type="text" class="form-control form-control-sm header-value"
                       value="${this.escapeHtml(value)}"
                       placeholder="Header value">
            </div>
            <div class="header-col-actions">
                <button class="btn btn-sm btn-danger delete-header-btn" type="button" title="Delete header">
                    <i class="fas fa-trash"></i>
                </button>
            </div>
        `;

        this.tableBody.appendChild(row);
        this.rows.push(row);

        // Event listeners
        const keyInput = row.querySelector('.header-key');
        const valueInput = row.querySelector('.header-value');
        const enabledCheckbox = row.querySelector('.header-enabled');
        const deleteBtn = row.querySelector('.delete-header-btn');

        // Auto-fill value when preset header selected
        keyInput.addEventListener('input', (e) => {
            const preset = this.presetHeaders.find(h => h.key === e.target.value);
            if (preset && !valueInput.value) {
                valueInput.value = preset.defaultValue || '';
            }
            this.onChange();
        });

        valueInput.addEventListener('input', () => this.onChange());

        enabledCheckbox.addEventListener('change', () => {
            row.dataset.enabled = enabledCheckbox.checked;
            this.onChange();
        });

        deleteBtn.addEventListener('click', () => {
            row.remove();
            this.rows = this.rows.filter(r => r !== row);

            // Add empty row if all deleted
            if (this.rows.length === 0) {
                this.addHeaderRow('', '', true);
            }

            this.onChange();
        });

        return row;
    }

    getHeaders() {
        const headers = {};
        this.rows.forEach(row => {
            const enabled = row.querySelector('.header-enabled').checked;
            const key = row.querySelector('.header-key').value.trim();
            const value = row.querySelector('.header-value').value.trim();

            if (enabled && key) {
                headers[key] = value;
            }
        });
        return headers;
    }

    setHeaders(headers) {
        this.headers = headers;
        this.tableBody.innerHTML = '';
        this.rows = [];

        Object.entries(headers).forEach(([key, value]) => {
            this.addHeaderRow(key, value, true);
        });

        if (this.rows.length === 0) {
            this.addHeaderRow('', '', true);
        }
    }

    onChange() {
        // Trigger custom event
        const event = new CustomEvent('headersChanged', {
            detail: { headers: this.getHeaders() }
        });
        this.container.dispatchEvent(event);
    }

    getPresetHeaders() {
        return [
            { key: 'Accept', description: 'Content types accepted', defaultValue: 'application/json' },
            { key: 'Content-Type', description: 'Request body content type', defaultValue: 'application/json' },
            { key: 'Authorization', description: 'Authentication credentials', defaultValue: 'Bearer ' },
            { key: 'User-Agent', description: 'Client identifier', defaultValue: 'ezHealthKonnect/1.0' },
            { key: 'X-API-Key', description: 'API Key authentication', defaultValue: '' },
            { key: 'Epic-Client-ID', description: 'Epic FHIR client ID', defaultValue: 'integration-engine' },
            { key: 'X-Request-ID', description: 'Request correlation ID', defaultValue: '{{message_id}}' },
            { key: 'Cache-Control', description: 'Caching directives', defaultValue: 'no-cache' },
            { key: 'Accept-Encoding', description: 'Compression support', defaultValue: 'gzip, deflate' },
            { key: 'X-Forwarded-For', description: 'Client IP address', defaultValue: '{{client_ip}}' }
        ];
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    injectStyles() {
        if (document.getElementById('header-builder-styles')) return;

        const style = document.createElement('style');
        style.id = 'header-builder-styles';
        style.textContent = `
            .header-builder {
                border: 1px solid #ddd;
                border-radius: 4px;
                padding: 15px;
                background: #f8f9fa;
            }

            .header-builder-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                margin-bottom: 15px;
            }

            .header-builder-header h4 {
                margin: 0;
                font-size: 16px;
                font-weight: 600;
            }

            .header-builder-table {
                background: white;
                border-radius: 4px;
                overflow: hidden;
            }

            .header-row {
                display: grid;
                grid-template-columns: 40px 1fr 1fr 60px;
                gap: 10px;
                padding: 8px 10px;
                border-bottom: 1px solid #eee;
                align-items: center;
            }

            .header-row-labels {
                background: #f1f3f5;
                font-weight: 600;
                font-size: 12px;
                text-transform: uppercase;
                color: #495057;
            }

            .header-row[data-enabled="false"] {
                opacity: 0.5;
            }

            .header-col-checkbox {
                text-align: center;
            }

            .header-col-key input,
            .header-col-value input {
                width: 100%;
            }

            .header-col-actions {
                text-align: center;
            }

            .delete-header-btn {
                padding: 4px 8px;
            }

            .add-header-btn {
                font-size: 12px;
                padding: 4px 12px;
            }

            .header-row:last-child {
                border-bottom: none;
            }

            .header-row:hover {
                background: #f8f9fa;
            }
        `;
        document.head.appendChild(style);
    }
}

// Export for use in other modules
if (typeof module !== 'undefined' && module.exports) {
    module.exports = HeaderBuilder;
}
