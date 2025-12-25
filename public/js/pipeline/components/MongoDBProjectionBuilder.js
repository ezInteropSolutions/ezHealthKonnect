/**
 * MongoDBProjectionBuilder Component - TAG-BASED DESIGN
 *
 * CREATIVE NO-CODE field selector for MongoDB projection
 * Uses tag-based interface with autocomplete for minimal clicks
 *
 * Features:
 * - Type field name → Autocomplete suggestions appear
 * - Press Enter or click suggestion → Field added as tag
 * - Click X on tag to remove
 * - Quick "Add All Common Fields" button
 * - Include/Exclude mode toggle
 * - Live JSON preview
 */

class MongoDBProjectionBuilder {
    constructor(container, initialProjection = {}, connectionConfig = {}) {
        this.container = container;
        this.projection = initialProjection;
        this.mode = this.detectMode(initialProjection);
        this.fields = this.parseProjectionToFields(initialProjection);
        this.connectionConfig = connectionConfig;
        this.commonFields = this.getCommonFields(); // Default fields
        this.dynamicFields = []; // Fields from actual collection
        this.loadingFields = false;
        this.render();
        this.loadCollectionSchema(); // Fetch real fields from MongoDB
    }

    /**
     * Detect if projection is include (1) or exclude (0) mode
     */
    detectMode(projection) {
        const values = Object.values(projection);
        if (values.length === 0) return 'include';
        return values[0] === 1 ? 'include' : 'exclude';
    }

    /**
     * Convert MongoDB projection to field list
     */
    parseProjectionToFields(projection) {
        return Object.keys(projection);
    }

    /**
     * Get list of common MongoDB fields with descriptions
     */
    getCommonFields() {
        return [
            { name: 'mrn', description: 'Medical Record Number', category: 'Identity' },
            { name: 'firstName', description: 'First Name', category: 'Demographics' },
            { name: 'lastName', description: 'Last Name', category: 'Demographics' },
            { name: 'dateOfBirth', description: 'Date of Birth', category: 'Demographics' },
            { name: 'gender', description: 'Gender', category: 'Demographics' },
            { name: 'ssn', description: 'Social Security Number', category: 'Identity' },
            { name: 'phone', description: 'Phone Number', category: 'Contact' },
            { name: 'email', description: 'Email Address', category: 'Contact' },
            { name: 'address', description: 'Full Address Object', category: 'Contact' },
            { name: 'address.street', description: 'Street Address', category: 'Contact' },
            { name: 'address.city', description: 'City', category: 'Contact' },
            { name: 'address.state', description: 'State', category: 'Contact' },
            { name: 'address.zip', description: 'ZIP Code', category: 'Contact' },
            { name: 'insurance', description: 'Insurance Object', category: 'Insurance' },
            { name: 'insurance.provider', description: 'Insurance Provider', category: 'Insurance' },
            { name: 'insurance.memberId', description: 'Member ID', category: 'Insurance' },
            { name: 'insurance.groupNumber', description: 'Group Number', category: 'Insurance' },
            { name: 'allergies', description: 'Allergies Array', category: 'Clinical' },
            { name: 'chronicConditions', description: 'Chronic Conditions', category: 'Clinical' },
            { name: 'primaryProvider', description: 'Primary Provider NPI', category: 'Clinical' },
            { name: 'lastVisit', description: 'Last Visit Date', category: 'Clinical' },
            { name: 'status', description: 'Patient Status', category: 'Administrative' },
            { name: 'facility', description: 'Facility Code', category: 'Administrative' }
        ];
    }

    generateId() {
        return 'field_' + Math.random().toString(36).substring(2, 11);
    }

    /**
     * Load collection schema from MongoDB backend
     */
    async loadCollectionSchema() {
        // Check if we have connection info and collection name
        const { dbHost, dbPort, dbName, dbUser, dbPassword, collection } = this.connectionConfig;

        if (!dbHost || !collection) {
            console.log('[MongoDBProjectionBuilder] No connection config or collection, using default fields');
            return;
        }

        this.loadingFields = true;
        this.showLoadingIndicator();

        try {
            const response = await fetch('/api/database/mongodb-schema', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    dbHost: dbHost || 'mongodb',
                    dbPort: dbPort || 27017,
                    dbName: dbName || 'ezhealthkonnect',
                    dbUser: dbUser || '',
                    dbPassword: dbPassword || '',
                    collection: collection
                })
            });

            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }

            const result = await response.json();

            if (result.success && result.fields) {
                this.dynamicFields = result.fields;
                console.log(`[MongoDBProjectionBuilder] Loaded ${result.count} fields from collection '${collection}'`);

                // Re-render autocomplete with dynamic fields
                this.hideLoadingIndicator();
            } else {
                throw new Error(result.error || 'Failed to load schema');
            }
        } catch (error) {
            console.error('[MongoDBProjectionBuilder] Failed to load collection schema:', error);
            this.hideLoadingIndicator();
            // Fall back to default fields
        } finally {
            this.loadingFields = false;
        }
    }

    showLoadingIndicator() {
        const input = this.container.querySelector('.field-input');
        if (input) {
            input.placeholder = 'Loading fields from collection...';
            input.disabled = true;
        }
    }

    hideLoadingIndicator() {
        const input = this.container.querySelector('.field-input');
        if (input) {
            input.placeholder = 'Type field name and press Enter... (e.g., firstName, insurance.provider)';
            input.disabled = false;
        }

        // Update "Select All" button text with field count
        this.updateSelectAllButton();
    }

    updateSelectAllButton() {
        const addAllBtn = this.container.querySelector('.add-all-btn');
        if (!addAllBtn) return;

        const availableFields = this.getAvailableFields();
        const remainingCount = availableFields.filter(f => !this.fields.includes(f.name)).length;

        if (this.dynamicFields.length > 0) {
            // Using dynamic fields from collection
            addAllBtn.innerHTML = `<i class="fas fa-check-double"></i> Select All (${availableFields.length})`;
            addAllBtn.title = `Add all ${availableFields.length} fields from the '${this.connectionConfig.collection}' collection`;
        } else {
            // Using default common fields
            addAllBtn.innerHTML = `<i class="fas fa-check-double"></i> Select All (${availableFields.length})`;
            addAllBtn.title = `Add all ${availableFields.length} common healthcare fields`;
        }
    }

    /**
     * Get available fields (dynamic fields if loaded, otherwise common fields)
     */
    getAvailableFields() {
        return this.dynamicFields.length > 0 ? this.dynamicFields : this.commonFields;
    }

    render() {
        this.container.innerHTML = `
            <div class="mongodb-projection-builder-v2">
                <div class="projection-header">
                    <div class="projection-title">
                        <i class="fas fa-tags"></i>
                        <span>Select Fields to Return</span>
                        <span class="help-icon" title="Type field names and press Enter to add. Click X to remove. Leave empty to return all fields.">
                            <i class="fas fa-question-circle"></i>
                        </span>
                    </div>
                </div>

                <div class="projection-mode-toggle">
                    <label class="mode-option ${this.mode === 'include' ? 'active' : ''}" data-mode="include">
                        <input type="radio" name="projection-mode-v2" value="include" ${this.mode === 'include' ? 'checked' : ''}>
                        <i class="fas fa-check-circle"></i>
                        <span>Include Only</span>
                        <small>Return only selected fields</small>
                    </label>
                    <label class="mode-option ${this.mode === 'exclude' ? 'active' : ''}" data-mode="exclude">
                        <input type="radio" name="projection-mode-v2" value="exclude" ${this.mode === 'exclude' ? 'checked' : ''}>
                        <i class="fas fa-times-circle"></i>
                        <span>Exclude</span>
                        <small>Return all except selected</small>
                    </label>
                </div>

                <div class="field-input-container">
                    <div class="field-input-wrapper">
                        <i class="fas fa-search input-icon"></i>
                        <input type="text"
                               class="field-input"
                               placeholder="Type field name and press Enter... (e.g., firstName, insurance.provider)"
                               autocomplete="off">
                        <div class="quick-actions">
                            <button type="button" class="btn-quick-action add-all-btn" title="Add all fields from collection">
                                <i class="fas fa-check-double"></i> Select All
                            </button>
                            <button type="button" class="btn-quick-action add-common-btn" title="Add common healthcare fields">
                                <i class="fas fa-plus-circle"></i> Add Common
                            </button>
                            <button type="button" class="btn-quick-action clear-all-btn" title="Clear all fields">
                                <i class="fas fa-trash"></i> Clear All
                            </button>
                        </div>
                    </div>
                    <div class="autocomplete-dropdown" style="display: none;"></div>
                </div>

                <div class="selected-fields-container">
                    <div class="selected-fields-tags"></div>
                    <div class="empty-state" style="${this.fields.length > 0 ? 'display: none;' : ''}">
                        <i class="fas fa-info-circle"></i>
                        <p>No fields selected</p>
                        <small>All fields will be returned from MongoDB</small>
                    </div>
                </div>

                <div class="projection-preview">
                    <div class="preview-header">
                        <label>Generated Projection</label>
                        <span class="preview-hint">Copy-paste friendly JSON</span>
                    </div>
                    <pre class="projection-json-preview"></pre>
                </div>
            </div>
        `;

        this.renderTags();
        this.attachEventListeners();
        this.updatePreview();
    }

    renderTags() {
        const container = this.container.querySelector('.selected-fields-tags');
        const emptyState = this.container.querySelector('.empty-state');

        if (this.fields.length === 0) {
            container.innerHTML = '';
            emptyState.style.display = 'block';
            return;
        }

        emptyState.style.display = 'none';

        let html = '';
        this.fields.forEach(fieldName => {
            const fieldInfo = this.commonFields.find(f => f.name === fieldName);
            const category = fieldInfo?.category || 'Custom';
            const categoryColor = this.getCategoryColor(category);

            html += `
                <div class="field-tag" data-field="${this.escapeHtml(fieldName)}" style="border-color: ${categoryColor};">
                    <span class="field-tag-category" style="background: ${categoryColor};">${category}</span>
                    <span class="field-tag-name">${this.escapeHtml(fieldName)}</span>
                    <button type="button" class="field-tag-remove" title="Remove field">
                        <i class="fas fa-times"></i>
                    </button>
                </div>
            `;
        });

        container.innerHTML = html;
    }

    getCategoryColor(category) {
        const colors = {
            'Identity': '#3b82f6',       // Blue
            'Demographics': '#8b5cf6',   // Purple
            'Contact': '#10b981',        // Green
            'Insurance': '#f59e0b',      // Amber
            'Clinical': '#ef4444',       // Red
            'Administrative': '#64748b', // Gray
            'Custom': '#06b6d4'          // Cyan
        };
        return colors[category] || colors['Custom'];
    }

    attachEventListeners() {
        const input = this.container.querySelector('.field-input');
        const dropdown = this.container.querySelector('.autocomplete-dropdown');
        const tagsContainer = this.container.querySelector('.selected-fields-tags');
        const modeRadios = this.container.querySelectorAll('[name="projection-mode-v2"]');
        const addAllBtn = this.container.querySelector('.add-all-btn');
        const addCommonBtn = this.container.querySelector('.add-common-btn');
        const clearAllBtn = this.container.querySelector('.clear-all-btn');

        // Input events
        input.addEventListener('input', (e) => this.handleInput(e, dropdown));
        input.addEventListener('keydown', (e) => this.handleKeydown(e, dropdown));
        input.addEventListener('blur', () => {
            // Delay to allow click on dropdown to register first
            setTimeout(() => dropdown.style.display = 'none', 300);
        });

        // Tag removal
        tagsContainer.addEventListener('click', (e) => {
            if (e.target.closest('.field-tag-remove')) {
                const tag = e.target.closest('.field-tag');
                const fieldName = tag.dataset.field;
                this.removeField(fieldName);
            }
        });

        // Mode toggle
        modeRadios.forEach(radio => {
            radio.addEventListener('change', (e) => {
                this.mode = e.target.value;

                // Update active class on labels
                this.container.querySelectorAll('.mode-option').forEach(label => {
                    label.classList.remove('active');
                });
                e.target.closest('.mode-option').classList.add('active');

                this.updatePreview();
            });
        });

        // Quick actions
        addAllBtn.addEventListener('click', () => this.addAllFields());
        addCommonBtn.addEventListener('click', () => this.addCommonFields());
        clearAllBtn.addEventListener('click', () => this.clearAll());

        // Dropdown item clicks - use mousedown to fire before blur event
        dropdown.addEventListener('mousedown', (e) => {
            e.preventDefault(); // Prevent input from losing focus
            const item = e.target.closest('.autocomplete-item');
            if (item) {
                const fieldName = item.dataset.field;
                this.addField(fieldName);
                input.value = '';
                dropdown.style.display = 'none';
                input.focus(); // Keep focus on input for continued typing
            }
        });
    }

    handleInput(e, dropdown) {
        const query = e.target.value.toLowerCase().trim();

        if (query.length === 0) {
            dropdown.style.display = 'none';
            return;
        }

        // Use dynamic fields if available, otherwise use common fields
        const availableFields = this.getAvailableFields();

        // Filter fields
        const matches = availableFields
            .filter(field => {
                const name = (field.name || '').toLowerCase();
                const description = (field.description || '').toLowerCase();
                const category = (field.category || '').toLowerCase();

                return name.includes(query) || description.includes(query) || category.includes(query);
            })
            .filter(field => !this.fields.includes(field.name)) // Exclude already selected
            .slice(0, 10); // Limit to 10 suggestions

        if (matches.length === 0) {
            dropdown.style.display = 'none';
            return;
        }

        // Render dropdown
        let html = '';
        matches.forEach(field => {
            const categoryColor = this.getCategoryColor(field.category);
            html += `
                <div class="autocomplete-item" data-field="${field.name}">
                    <span class="autocomplete-category" style="background: ${categoryColor};">${field.category}</span>
                    <div class="autocomplete-content">
                        <span class="autocomplete-field">${this.highlightMatch(field.name, query)}</span>
                        <span class="autocomplete-desc">${field.description}</span>
                    </div>
                </div>
            `;
        });

        dropdown.innerHTML = html;
        dropdown.style.display = 'block';
    }

    handleKeydown(e, dropdown) {
        if (e.key === 'Enter') {
            e.preventDefault();
            const query = e.target.value.trim();

            // Check if there's an active dropdown item
            const activeItem = dropdown.querySelector('.autocomplete-item.active');
            if (activeItem) {
                const fieldName = activeItem.dataset.field;
                this.addField(fieldName);
            } else if (query) {
                // Add the typed value directly
                this.addField(query);
            }

            e.target.value = '';
            dropdown.style.display = 'none';
        } else if (e.key === 'Escape') {
            dropdown.style.display = 'none';
        } else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
            e.preventDefault();
            this.navigateDropdown(dropdown, e.key === 'ArrowDown' ? 1 : -1);
        }
    }

    navigateDropdown(dropdown, direction) {
        const items = dropdown.querySelectorAll('.autocomplete-item');
        if (items.length === 0) return;

        const activeItem = dropdown.querySelector('.autocomplete-item.active');
        let newIndex = 0;

        if (activeItem) {
            activeItem.classList.remove('active');
            const currentIndex = Array.from(items).indexOf(activeItem);
            newIndex = currentIndex + direction;
            if (newIndex < 0) newIndex = items.length - 1;
            if (newIndex >= items.length) newIndex = 0;
        }

        items[newIndex].classList.add('active');
        items[newIndex].scrollIntoView({ block: 'nearest' });
    }

    highlightMatch(text, query) {
        const index = text.toLowerCase().indexOf(query.toLowerCase());
        if (index === -1) return this.escapeHtml(text);

        const before = this.escapeHtml(text.substring(0, index));
        const match = this.escapeHtml(text.substring(index, index + query.length));
        const after = this.escapeHtml(text.substring(index + query.length));

        return `${before}<mark>${match}</mark>${after}`;
    }

    addField(fieldName) {
        if (!fieldName || this.fields.includes(fieldName)) return;

        this.fields.push(fieldName);
        this.renderTags();
        this.updatePreview();
        this.updateSelectAllButton();
    }

    removeField(fieldName) {
        this.fields = this.fields.filter(f => f !== fieldName);
        this.renderTags();
        this.updatePreview();
        this.updateSelectAllButton();
    }

    addAllFields() {
        // Add all available fields (from collection schema if loaded, otherwise common fields)
        const availableFields = this.getAvailableFields();

        let addedCount = 0;
        availableFields.forEach(field => {
            if (!this.fields.includes(field.name)) {
                this.fields.push(field.name);
                addedCount++;
            }
        });

        if (addedCount > 0) {
            console.log(`[MongoDBProjectionBuilder] Added ${addedCount} fields (total: ${this.fields.length})`);
            this.renderTags();
            this.updatePreview();
            this.updateSelectAllButton();
        } else {
            console.log('[MongoDBProjectionBuilder] All fields already selected');
        }
    }

    addCommonFields() {
        // Add top 10 most common healthcare fields
        const commonFieldNames = [
            'mrn', 'firstName', 'lastName', 'dateOfBirth', 'gender',
            'phone', 'email', 'address', 'insurance', 'allergies'
        ];

        let addedCount = 0;
        commonFieldNames.forEach(field => {
            if (!this.fields.includes(field)) {
                this.fields.push(field);
                addedCount++;
            }
        });

        if (addedCount > 0) {
            console.log(`[MongoDBProjectionBuilder] Added ${addedCount} common fields`);
            this.renderTags();
            this.updatePreview();
            this.updateSelectAllButton();
        }
    }

    clearAll() {
        if (this.fields.length === 0) return;

        if (confirm('Remove all selected fields?')) {
            this.fields = [];
            this.renderTags();
            this.updatePreview();
            this.updateSelectAllButton();
        }
    }

    updatePreview() {
        const projection = this.buildProjection();
        const preview = this.container.querySelector('.projection-json-preview');

        if (Object.keys(projection).length === 0) {
            preview.textContent = '{}  // Empty = Return all fields';
            preview.classList.add('empty');
        } else {
            preview.textContent = JSON.stringify(projection, null, 2);
            preview.classList.remove('empty');
        }
    }

    buildProjection() {
        const projection = {};
        const value = this.mode === 'include' ? 1 : 0;

        this.fields.forEach(field => {
            projection[field] = value;
        });

        return projection;
    }

    getProjection() {
        return this.buildProjection();
    }

    escapeHtml(text) {
        const map = {
            '&': '&amp;',
            '<': '&lt;',
            '>': '&gt;',
            '"': '&quot;',
            "'": '&#039;'
        };
        return String(text).replace(/[&<>"']/g, m => map[m]);
    }
}
