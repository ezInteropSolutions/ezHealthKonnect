/**
 * ================================================================
 * METADATA BUILDER COMPONENT
 * ================================================================
 * Version: 1.1 - Fixed nested JSON display
 *
 * User-friendly key-value pair builder for custom metadata
 * Similar to ValidationRuleBuilder but simpler (just key-value pairs)
 *
 * Features:
 * - Add/remove key-value pairs via UI
 * - Import/export JSON for technical users
 * - Real-time validation
 * - Common metadata suggestions
 * - Proper display of nested JSON objects
 */

class MetadataBuilder {
    constructor(container, initialMetadata = {}) {
        this.container = container;
        this.metadata = initialMetadata;
        this.metadataArray = this.convertToArray(initialMetadata);

        this.render();
    }

    /**
     * Convert metadata object to array for easier manipulation
     */
    convertToArray(metadata) {
        return Object.entries(metadata).map(([key, value]) => ({ key, value }));
    }

    /**
     * Convert metadata array back to object
     */
    convertToObject(metadataArray) {
        const obj = {};
        metadataArray.forEach(item => {
            if (item.key && item.key.trim()) {
                obj[item.key.trim()] = item.value;
            }
        });
        return obj;
    }

    /**
     * Get current metadata as object
     */
    getMetadata() {
        // Collect from form
        this.collectFormData();
        return this.convertToObject(this.metadataArray);
    }

    /**
     * Collect data from form inputs
     */
    collectFormData() {
        const rows = this.container.querySelectorAll('.metadata-row');
        this.metadataArray = [];

        rows.forEach(row => {
            const keyInput = row.querySelector('.metadata-key');
            const valueInput = row.querySelector('.metadata-value');

            if (keyInput && valueInput) {
                const key = keyInput.value.trim();
                let value = valueInput.value;

                // Try to parse as JSON if it looks like JSON
                if (value && (value.trim().startsWith('{') || value.trim().startsWith('['))) {
                    try {
                        value = JSON.parse(value);
                    } catch (e) {
                        // Keep as string if parse fails
                    }
                }

                if (key) { // Only add if key is not empty
                    this.metadataArray.push({ key, value });
                }
            }
        });
    }

    /**
     * Render the metadata builder UI
     */
    render() {
        this.container.innerHTML = `
            <div class="metadata-builder">
                <!-- Header -->
                <div class="metadata-builder-header" style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem; padding: 0.75rem; background: #f3f4f6; border-radius: 6px;">
                    <div>
                        <h4 style="margin: 0; color: #1e3a8a; font-size: 0.95rem;">
                            <i class="fas fa-tags"></i> Custom Metadata Fields
                        </h4>
                        <p style="margin: 0.25rem 0 0 0; color: #6b7280; font-size: 0.8rem;">Add key-value pairs for custom metadata</p>
                    </div>
                    <div style="display: flex; gap: 0.5rem;">
                        <button type="button" class="btn-icon" id="importMetadataBtn" title="Import from JSON">
                            <i class="fas fa-file-import"></i>
                        </button>
                        <button type="button" class="btn-icon" id="exportMetadataBtn" title="Export to JSON">
                            <i class="fas fa-file-export"></i>
                        </button>
                        <button type="button" class="btn-icon" id="addSuggestedBtn" title="Add Common Fields">
                            <i class="fas fa-lightbulb"></i>
                        </button>
                    </div>
                </div>

                <!-- Metadata Rows Container -->
                <div class="metadata-rows-container" style="max-height: 400px; overflow-y: auto; margin-bottom: 1rem;">
                    ${this.renderRows()}
                </div>

                <!-- Add Row Button -->
                <button type="button" class="btn-add-metadata" id="addMetadataRowBtn" style="width: 100%; padding: 0.5rem; border: 2px dashed #cbd5e1; background: #f9fafb; color: #475569; border-radius: 6px; cursor: pointer; font-size: 0.875rem; transition: all 0.2s;">
                    <i class="fas fa-plus"></i> Add Metadata Field
                </button>

                <!-- JSON Import/Export Modal -->
                <div id="metadataJsonModal" class="metadata-json-modal" style="display: none; position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.5); z-index: 10000; align-items: center; justify-content: center;">
                    <div style="background: white; padding: 1.5rem; border-radius: 8px; max-width: 600px; width: 90%;">
                        <h4 style="margin: 0 0 1rem 0; color: #1e3a8a;"><i class="fas fa-code"></i> <span id="modalTitle">Import/Export JSON</span></h4>
                        <textarea id="metadataJsonTextarea" rows="12" style="width: 100%; padding: 0.75rem; border: 1px solid #cbd5e1; border-radius: 4px; font-family: monospace; font-size: 0.875rem;" placeholder='{"environment": "production", "version": "1.0"}'></textarea>
                        <div style="margin-top: 1rem; display: flex; gap: 0.5rem; justify-content: flex-end;">
                            <button type="button" class="btn btn-secondary" id="cancelJsonBtn">Cancel</button>
                            <button type="button" class="btn btn-primary" id="confirmJsonBtn">Confirm</button>
                        </div>
                    </div>
                </div>

                <!-- Suggestions Modal -->
                <div id="metadataSuggestionsModal" class="metadata-suggestions-modal" style="display: none; position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.5); z-index: 10000; align-items: center; justify-content: center;">
                    <div style="background: white; padding: 1.5rem; border-radius: 8px; max-width: 500px; width: 90%;">
                        <h4 style="margin: 0 0 1rem 0; color: #1e3a8a;"><i class="fas fa-lightbulb"></i> Common Metadata Fields</h4>
                        <p style="color: #6b7280; font-size: 0.875rem; margin-bottom: 1rem;">Select fields to add to your metadata:</p>
                        <div class="suggestions-list">
                            ${this.renderSuggestions()}
                        </div>
                        <div style="margin-top: 1rem; display: flex; gap: 0.5rem; justify-content: flex-end;">
                            <button type="button" class="btn btn-secondary" id="cancelSuggestionsBtn">Cancel</button>
                            <button type="button" class="btn btn-primary" id="addSelectedSuggestionsBtn">Add Selected</button>
                        </div>
                    </div>
                </div>
            </div>
        `;

        this.attachEvents();
    }

    /**
     * Render metadata rows
     */
    renderRows() {
        if (this.metadataArray.length === 0) {
            return `
                <div class="empty-state" style="text-align: center; padding: 2rem; color: #9ca3af;">
                    <i class="fas fa-inbox" style="font-size: 2rem; margin-bottom: 0.5rem;"></i>
                    <p>No metadata fields added yet.</p>
                    <p style="font-size: 0.875rem;">Click "Add Metadata Field" to get started.</p>
                </div>
            `;
        }

        return this.metadataArray.map((item, index) => this.renderRow(item, index)).join('');
    }

    /**
     * Render a single metadata row
     */
    renderRow(item, index) {
        // Check if value is an object/array
        const isObject = typeof item.value === 'object' && item.value !== null;
        const displayValue = isObject ? JSON.stringify(item.value, null, 2) : (item.value || '');
        const escapedValue = this.escapeHtml(displayValue);

        return `
            <div class="metadata-row" data-index="${index}" style="display: flex; gap: 0.5rem; margin-bottom: 0.5rem; padding: 0.75rem; background: white; border: 1px solid #e5e7eb; border-radius: 6px; align-items: flex-start;">
                <div style="flex: 1; min-width: 0;">
                    <label style="display: block; font-size: 0.75rem; color: #6b7280; margin-bottom: 0.25rem;">Key</label>
                    <input type="text" class="metadata-key" value="${this.escapeHtml(item.key || '')}" placeholder="e.g., environment" style="width: 100%; padding: 0.5rem; border: 1px solid #cbd5e1; border-radius: 4px; font-size: 0.875rem;">
                </div>
                <div style="flex: 2; min-width: 0;">
                    <label style="display: block; font-size: 0.75rem; color: #6b7280; margin-bottom: 0.25rem;">
                        Value ${isObject ? '<span style="color: #059669; font-weight: 500;">(JSON Object)</span>' : ''}
                    </label>
                    ${isObject ?
                        `<textarea class="metadata-value" rows="6" placeholder="e.g., production" style="width: 100%; padding: 0.5rem; border: 1px solid #cbd5e1; border-radius: 4px; font-size: 0.875rem; font-family: 'Courier New', monospace; resize: vertical;">${escapedValue}</textarea>` :
                        `<input type="text" class="metadata-value" value="${escapedValue}" placeholder="e.g., production" style="width: 100%; padding: 0.5rem; border: 1px solid #cbd5e1; border-radius: 4px; font-size: 0.875rem;">`
                    }
                </div>
                <div style="padding-top: 1.5rem;">
                    <button type="button" class="btn-delete-metadata" data-index="${index}" title="Delete" style="padding: 0.5rem 0.75rem; background: #fee; color: #dc2626; border: 1px solid #fecaca; border-radius: 4px; cursor: pointer; transition: all 0.2s;">
                        <i class="fas fa-trash"></i>
                    </button>
                </div>
            </div>
        `;
    }

    /**
     * Escape HTML to prevent XSS
     */
    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    /**
     * Render common metadata suggestions
     */
    renderSuggestions() {
        const suggestions = [
            { key: 'environment', value: 'production', description: 'Deployment environment (production, staging, dev)' },
            { key: 'processingNode', value: 'server-01', description: 'Server/node that processed the message' },
            { key: 'facility', value: 'MAIN_CAMPUS', description: 'Healthcare facility identifier' },
            { key: 'department', value: 'RADIOLOGY', description: 'Department or organizational unit' },
            { key: 'version', value: '2.1.0', description: 'Application or integration version' },
            { key: 'region', value: 'us-east-1', description: 'Geographic region or data center' },
            { key: 'tenant', value: 'hospital-001', description: 'Multi-tenant identifier' },
            { key: 'source', value: 'EPIC', description: 'Source system name' }
        ];

        return suggestions.map(suggestion => `
            <label class="suggestion-item" style="display: flex; align-items: flex-start; padding: 0.75rem; border: 1px solid #e5e7eb; border-radius: 4px; margin-bottom: 0.5rem; cursor: pointer; transition: all 0.2s;">
                <input type="checkbox" class="suggestion-checkbox" data-key="${suggestion.key}" data-value="${suggestion.value}" style="margin-right: 0.75rem; margin-top: 0.25rem;">
                <div style="flex: 1;">
                    <div style="font-weight: 500; color: #1e3a8a; margin-bottom: 0.25rem;">
                        <code style="background: #e0e7ff; padding: 0.125rem 0.375rem; border-radius: 3px; font-size: 0.875rem;">${suggestion.key}</code>
                        <span style="color: #6b7280; font-weight: normal; margin-left: 0.5rem;">= "${suggestion.value}"</span>
                    </div>
                    <p style="margin: 0; color: #6b7280; font-size: 0.8rem;">${suggestion.description}</p>
                </div>
            </label>
        `).join('');
    }

    /**
     * Attach event listeners
     */
    attachEvents() {
        // Add row button
        const addBtn = this.container.querySelector('#addMetadataRowBtn');
        if (addBtn) {
            addBtn.addEventListener('click', () => this.addRow());
        }

        // Delete buttons
        this.container.querySelectorAll('.btn-delete-metadata').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const index = parseInt(e.currentTarget.dataset.index);
                this.deleteRow(index);
            });
        });

        // Import/Export buttons
        const importBtn = this.container.querySelector('#importMetadataBtn');
        if (importBtn) {
            importBtn.addEventListener('click', () => this.showImportModal());
        }

        const exportBtn = this.container.querySelector('#exportMetadataBtn');
        if (exportBtn) {
            exportBtn.addEventListener('click', () => this.showExportModal());
        }

        // Suggestions button
        const suggestionsBtn = this.container.querySelector('#addSuggestedBtn');
        if (suggestionsBtn) {
            suggestionsBtn.addEventListener('click', () => this.showSuggestionsModal());
        }

        // Modal buttons
        const cancelJsonBtn = this.container.querySelector('#cancelJsonBtn');
        if (cancelJsonBtn) {
            cancelJsonBtn.addEventListener('click', () => this.hideJsonModal());
        }

        const confirmJsonBtn = this.container.querySelector('#confirmJsonBtn');
        if (confirmJsonBtn) {
            confirmJsonBtn.addEventListener('click', () => this.confirmJsonImport());
        }

        const cancelSuggestionsBtn = this.container.querySelector('#cancelSuggestionsBtn');
        if (cancelSuggestionsBtn) {
            cancelSuggestionsBtn.addEventListener('click', () => this.hideSuggestionsModal());
        }

        const addSelectedSuggestionsBtn = this.container.querySelector('#addSelectedSuggestionsBtn');
        if (addSelectedSuggestionsBtn) {
            addSelectedSuggestionsBtn.addEventListener('click', () => this.addSelectedSuggestions());
        }
    }

    /**
     * Add a new row
     */
    addRow(key = '', value = '') {
        this.metadataArray.push({ key, value });
        this.render();
    }

    /**
     * Delete a row
     */
    deleteRow(index) {
        this.metadataArray.splice(index, 1);
        this.render();
    }

    /**
     * Show import modal
     */
    showImportModal() {
        const modal = this.container.querySelector('#metadataJsonModal');
        const textarea = this.container.querySelector('#metadataJsonTextarea');
        const title = this.container.querySelector('#modalTitle');

        if (modal && textarea && title) {
            title.textContent = 'Import Metadata from JSON';
            textarea.value = '';
            modal.style.display = 'flex';
        }
    }

    /**
     * Show export modal
     */
    showExportModal() {
        const modal = this.container.querySelector('#metadataJsonModal');
        const textarea = this.container.querySelector('#metadataJsonTextarea');
        const title = this.container.querySelector('#modalTitle');

        if (modal && textarea && title) {
            title.textContent = 'Export Metadata as JSON';
            this.collectFormData();
            const metadata = this.convertToObject(this.metadataArray);
            textarea.value = JSON.stringify(metadata, null, 2);
            modal.style.display = 'flex';
        }
    }

    /**
     * Hide JSON modal
     */
    hideJsonModal() {
        const modal = this.container.querySelector('#metadataJsonModal');
        if (modal) {
            modal.style.display = 'none';
        }
    }

    /**
     * Confirm JSON import
     */
    confirmJsonImport() {
        const textarea = this.container.querySelector('#metadataJsonTextarea');

        if (textarea) {
            try {
                const imported = JSON.parse(textarea.value);

                if (typeof imported !== 'object' || Array.isArray(imported)) {
                    this.showNotification('Invalid JSON: Expected an object with key-value pairs', 'error');
                    return;
                }

                this.metadataArray = this.convertToArray(imported);
                this.render();
                this.hideJsonModal();
            } catch (error) {
                this.showNotification('Invalid JSON: ' + error.message, 'error');
            }
        }
    }

    /**
     * Show in-app notification
     */
    showNotification(message, type = 'info') {
        const notification = document.createElement('div');
        notification.style.cssText = `
            position: fixed;
            bottom: 20px;
            right: 20px;
            background: ${type === 'success' ? '#10b981' : type === 'error' ? '#ef4444' : type === 'warning' ? '#f59e0b' : '#06b6d4'};
            color: white;
            padding: 12px 20px;
            border-radius: 6px;
            box-shadow: 0 4px 6px rgba(0, 0, 0, 0.1);
            z-index: 10000;
        `;
        notification.textContent = message;
        document.body.appendChild(notification);
        setTimeout(() => notification.remove(), 4000);
    }

    /**
     * Show suggestions modal
     */
    showSuggestionsModal() {
        const modal = this.container.querySelector('#metadataSuggestionsModal');
        if (modal) {
            modal.style.display = 'flex';
        }
    }

    /**
     * Hide suggestions modal
     */
    hideSuggestionsModal() {
        const modal = this.container.querySelector('#metadataSuggestionsModal');
        if (modal) {
            modal.style.display = 'none';
        }
    }

    /**
     * Add selected suggestions
     */
    addSelectedSuggestions() {
        const checkboxes = this.container.querySelectorAll('.suggestion-checkbox:checked');

        checkboxes.forEach(checkbox => {
            const key = checkbox.dataset.key;
            const value = checkbox.dataset.value;
            this.addRow(key, value);
        });

        this.hideSuggestionsModal();
    }
}

// Make globally available
window.MetadataBuilder = MetadataBuilder;
