/**
 * FieldPathSearchComponent - Reusable HL7 field path search/autocomplete component
 *
 * Features:
 * - Smart autocomplete with search by name, description, or path
 * - Category-based grouping (Patient, Visit, Insurance, etc.)
 * - Keyboard navigation (Arrow keys, Enter, Escape)
 * - Recent/frequently used suggestions
 * - Custom field support
 *
 * Usage:
 * const searchComponent = new FieldPathSearchComponent(inputElement, {
 *     onSelect: (fieldPath) => { console.log('Selected:', fieldPath); },
 *     placeholder: 'Search HL7 fields... (e.g., Patient MRN, PID-3)',
 *     allowCustom: true,
 *     showCategories: true
 * });
 */

class FieldPathSearchComponent {
    constructor(inputElement, options = {}) {
        console.log('[FieldPathSearchComponent] Initializing with input:', inputElement);
        this.input = inputElement;
        this.options = {
            onSelect: options.onSelect || (() => {}),
            placeholder: options.placeholder || 'Search HL7 fields...',
            allowCustom: options.allowCustom !== false, // Default true
            showCategories: options.showCategories !== false, // Default true
            maxSuggestions: options.maxSuggestions || 10,
            caseSensitive: options.caseSensitive || false,
            // NEW: Additional fields to include (e.g., step output variables)
            // Format: [{ name: 'var_name', path: 'steps.step_alias.var_name', description: '...', category: 'Step Outputs' }]
            additionalFields: options.additionalFields || [],
            // NEW: Callback to dynamically fetch step variables
            getStepVariables: options.getStepVariables || null,
            // NEW: Include predefined HL7 fields (default true)
            includeHL7Fields: options.includeHL7Fields !== false,
            ...options
        };

        this.dropdown = null;
        this.activeIndex = -1;
        this.recentPaths = this.loadRecentPaths();
        // Cache for dynamically loaded step variables
        this.cachedStepVariables = null;

        this.initialize();
    }

    initialize() {
        // Set input placeholder
        this.input.placeholder = this.options.placeholder;
        this.input.autocomplete = 'off';

        // Inject CSS styles if not already present
        this.injectStyles();

        // Create dropdown container
        this.createDropdown();

        // Attach event listeners
        this.attachEventListeners();
    }

    createDropdown() {
        // Create dropdown element
        this.dropdown = document.createElement('div');
        this.dropdown.className = 'field-path-search-dropdown';
        this.dropdown.style.display = 'none';

        // Use FIXED positioning to escape overflow: hidden constraints
        this.dropdown.style.position = 'fixed';
        this.dropdown.style.zIndex = '10000';

        // Append to body to avoid parent overflow clipping
        document.body.appendChild(this.dropdown);
    }

    attachEventListeners() {
        // Input events
        this.input.addEventListener('input', (e) => this.handleInput(e));
        this.input.addEventListener('focus', (e) => this.handleFocus(e));
        this.input.addEventListener('keydown', (e) => this.handleKeydown(e));
        this.input.addEventListener('blur', (e) => this.handleBlur(e));

        // Dropdown clicks - use mousedown to fire before blur
        this.dropdown.addEventListener('mousedown', (e) => {
            e.preventDefault(); // Prevent input from losing focus
            e.stopPropagation(); // Stop event from bubbling
            const item = e.target.closest('.field-path-item');
            if (item) {
                const fieldPath = item.dataset.path;
                this.selectField(fieldPath);
            }
        });

        // Click on dropdown should not close it
        this.dropdown.addEventListener('click', (e) => {
            e.stopPropagation();
        });

        // Store bound handler for cleanup
        this._documentClickHandler = (e) => {
            // Only process if dropdown is visible
            if (this.dropdown.style.display === 'none') return;

            // Don't close if clicking on input or dropdown
            if (this.input.contains(e.target) || this.dropdown.contains(e.target)) {
                return;
            }

            this.hideDropdown();
        };

        // Use capture phase to handle before other handlers
        document.addEventListener('click', this._documentClickHandler, true);
    }

    /**
     * Cleanup method to remove event listeners when component is destroyed
     */
    destroy() {
        if (this._documentClickHandler) {
            document.removeEventListener('click', this._documentClickHandler, true);
        }
        if (this.dropdown && this.dropdown.parentNode) {
            this.dropdown.parentNode.removeChild(this.dropdown);
        }
    }

    handleInput(e) {
        const query = this.input.value.trim();
        console.log('[FieldPathSearchComponent] Input event, query:', query);

        if (query.length === 0) {
            this.showRecentPaths();
            return;
        }

        this.searchFields(query);
    }

    handleFocus(e) {
        if (this.input.value.trim().length === 0) {
            this.showBrowseMode();
        } else {
            this.searchFields(this.input.value.trim());
        }
    }

    handleKeydown(e) {
        if (this.dropdown.style.display === 'none') return;

        const items = this.dropdown.querySelectorAll('.field-path-item');

        switch (e.key) {
            case 'ArrowDown':
                e.preventDefault();
                this.activeIndex = Math.min(this.activeIndex + 1, items.length - 1);
                this.updateActiveItem(items);
                break;

            case 'ArrowUp':
                e.preventDefault();
                this.activeIndex = Math.max(this.activeIndex - 1, 0);
                this.updateActiveItem(items);
                break;

            case 'Enter':
                e.preventDefault();
                if (this.activeIndex >= 0 && items[this.activeIndex]) {
                    const fieldPath = items[this.activeIndex].dataset.path;
                    this.selectField(fieldPath);
                } else if (this.options.allowCustom && this.input.value.trim()) {
                    this.selectField(this.input.value.trim());
                }
                break;

            case 'Escape':
                e.preventDefault();
                this.hideDropdown();
                break;

            case 'Tab':
                // Allow tab to select if there's an active item
                if (this.activeIndex >= 0 && items[this.activeIndex]) {
                    e.preventDefault();
                    const fieldPath = items[this.activeIndex].dataset.path;
                    this.selectField(fieldPath);
                }
                break;
        }
    }

    handleBlur(e) {
        // Delay to allow dropdown click to register
        setTimeout(() => {
            if (!this.dropdown.contains(document.activeElement)) {
                this.hideDropdown();
            }
        }, 200);
    }

    searchFields(query) {
        console.log('[FieldPathSearchComponent] Searching for:', query);
        const lowerQuery = this.options.caseSensitive ? query : query.toLowerCase();
        const fields = this.getCommonFields();
        console.log('[FieldPathSearchComponent] Total fields available:', fields.length);

        // Filter and rank matches
        const matches = fields
            .map(field => {
                const name = this.options.caseSensitive ? field.name : field.name.toLowerCase();
                const path = this.options.caseSensitive ? field.path : field.path.toLowerCase();
                const desc = this.options.caseSensitive ? field.description : field.description.toLowerCase();

                let score = 0;

                // Exact match in path (highest priority)
                if (path === lowerQuery) score = 100;
                // Starts with query in path
                else if (path.startsWith(lowerQuery)) score = 90;
                // Contains query in path
                else if (path.includes(lowerQuery)) score = 70;
                // Exact match in name
                else if (name === lowerQuery) score = 80;
                // Starts with query in name
                else if (name.startsWith(lowerQuery)) score = 60;
                // Contains query in name
                else if (name.includes(lowerQuery)) score = 50;
                // Contains query in description
                else if (desc.includes(lowerQuery)) score = 30;

                return { ...field, score };
            })
            .filter(field => field.score > 0)
            .sort((a, b) => b.score - a.score)
            .slice(0, this.options.maxSuggestions);

        if (matches.length > 0) {
            this.renderDropdown(matches, query);
        } else if (this.options.allowCustom) {
            this.showCustomOption(query);
        } else {
            this.hideDropdown();
        }
    }

    renderDropdown(fields, query = '') {
        this.activeIndex = -1;

        let html = '';

        if (this.options.showCategories) {
            // Group by category
            const grouped = this.groupByCategory(fields);

            for (const [category, items] of Object.entries(grouped)) {
                if (items.length > 0) {
                    html += `<div class="field-path-category">${category}</div>`;
                    items.forEach(field => {
                        html += this.renderFieldItem(field, query);
                    });
                }
            }
        } else {
            // Flat list
            fields.forEach(field => {
                html += this.renderFieldItem(field, query);
            });
        }

        // Add custom option if allowed and query exists
        if (this.options.allowCustom && query) {
            html += `
                <div class="field-path-item custom-path" data-path="${this.escapeHtml(query)}">
                    <div class="field-path-icon">✏️</div>
                    <div class="field-path-content">
                        <div class="field-path-name">Custom: ${this.highlightMatch(query, query)}</div>
                        <div class="field-path-desc">Use custom field path</div>
                    </div>
                </div>
            `;
        }

        this.dropdown.innerHTML = html;
        this.showDropdown();
    }

    renderFieldItem(field, query) {
        const categoryColor = this.getCategoryColor(field.category);
        const highlightedName = this.highlightMatch(field.name, query);
        const highlightedPath = this.highlightMatch(field.path, query);

        return `
            <div class="field-path-item" data-path="${this.escapeHtml(field.path)}">
                <div class="field-path-icon" style="background: ${categoryColor};">
                    ${this.getCategoryIcon(field.category)}
                </div>
                <div class="field-path-content">
                    <div class="field-path-name">${highlightedName}</div>
                    <div class="field-path-path">${highlightedPath}</div>
                    <div class="field-path-desc">${field.description}</div>
                </div>
            </div>
        `;
    }

    showRecentPaths() {
        if (this.recentPaths.length === 0) {
            this.hideDropdown();
            return;
        }

        const fields = this.getCommonFields();
        const recentFields = this.recentPaths
            .map(path => fields.find(f => f.path === path))
            .filter(f => f)
            .slice(0, 5);

        if (recentFields.length > 0) {
            let html = '<div class="field-path-category">⏱️ Recently Used</div>';
            recentFields.forEach(field => {
                html += this.renderFieldItem(field, '');
            });

            this.dropdown.innerHTML = html;
            this.showDropdown();
        }
    }

    /**
     * Browse mode: shown when the input is focused and empty.
     * Displays step-output leaf paths from the last test run (or backend
     * declarations) first, then falls back to recently-used paths.
     * This is the "no-code" experience — users can browse available fields
     * without knowing any path syntax.
     */
    showBrowseMode() {
        let html = '';

        // 1. Recently used paths (always shown first)
        const allFields = this.getAllFields();
        const recentFields = this.recentPaths
            .map(path => allFields.find(f => f.path === path))
            .filter(f => f)
            .slice(0, 5);

        if (recentFields.length > 0) {
            html += '<div class="field-path-category">⏱️ Recently Used</div>';
            recentFields.forEach(field => { html += this.renderFieldItem(field, ''); });
        }

        // 2. Step output variables — leaf paths only (skip array container hints)
        if (this.options.getStepVariables) {
            const stepVars = this.options.getStepVariables();
            if (Array.isArray(stepVars) && stepVars.length > 0) {
                // isArray=true marks container hints like "entry[]" — skip them
                const leafVars = stepVars.filter(v => !v.isArray);

                if (leafVars.length > 0) {
                    const limit = 30;
                    html += '<div class="field-path-category">📦 Available Step Outputs</div>';
                    leafVars.slice(0, limit).forEach(v => {
                        html += this.renderFieldItem({
                            ...v,
                            category: v.category || 'Step Outputs'
                        }, '');
                    });
                    if (leafVars.length > limit) {
                        html += `<div class="field-path-hint">Type to search ${leafVars.length - limit} more fields…</div>`;
                    }
                } else if (recentFields.length === 0) {
                    // Only arrays/containers available — show them with a hint
                    html += '<div class="field-path-category">📦 Available Step Outputs</div>';
                    stepVars.slice(0, 15).forEach(v => {
                        html += this.renderFieldItem({ ...v, category: v.category || 'Step Outputs' }, '');
                    });
                }
            } else if (recentFields.length === 0) {
                // No step variables and no recents — clickable hint to open test modal
                html += `
                    <div class="field-path-hint" style="padding:12px;text-align:center;color:#888;">
                        💡 <a href="#" style="color:#4a90e2;text-decoration:none;"
                              onmousedown="event.preventDefault();document.getElementById('testPipelineBtn')?.click();">
                            Run the test pipeline
                        </a> once to browse all available fields here,<br>
                        or type a path directly like <code>PID.5.1</code>
                    </div>`;
            }
        } else if (recentFields.length === 0) {
            // No callback and no recents — just hide
            this.hideDropdown();
            return;
        }

        if (html) {
            this.dropdown.innerHTML = html;
            this.showDropdown();
        }
    }

    showCustomOption(value) {
        this.dropdown.innerHTML = `
            <div class="field-path-item custom-path" data-path="${this.escapeHtml(value)}">
                <div class="field-path-icon">✏️</div>
                <div class="field-path-content">
                    <div class="field-path-name">Custom: ${this.escapeHtml(value)}</div>
                    <div class="field-path-desc">Use this custom field path</div>
                </div>
            </div>
        `;
        this.showDropdown();
    }

    updateActiveItem(items) {
        items.forEach((item, index) => {
            if (index === this.activeIndex) {
                item.classList.add('active');
                item.scrollIntoView({ block: 'nearest' });
            } else {
                item.classList.remove('active');
            }
        });
    }

    selectField(fieldPath) {
        // Add to recent paths
        this.addRecentPath(fieldPath);

        // Call onSelect callback
        this.options.onSelect(fieldPath);

        // Update input value
        this.input.value = fieldPath;

        // Hide dropdown
        this.hideDropdown();

        // Dispatch change event
        this.input.dispatchEvent(new Event('change', { bubbles: true }));
    }

    showDropdown() {
        console.log('[FieldPathSearchComponent] Showing dropdown');

        // Get input position relative to viewport (for fixed positioning)
        const rect = this.input.getBoundingClientRect();

        // Position dropdown below input using fixed positioning
        this.dropdown.style.top = `${rect.bottom + 4}px`;
        this.dropdown.style.left = `${rect.left}px`;
        this.dropdown.style.width = `${rect.width}px`;

        this.dropdown.style.display = 'block';

        console.log('[FieldPathSearchComponent] Positioned dropdown at:', {
            top: this.dropdown.style.top,
            left: this.dropdown.style.left,
            width: this.dropdown.style.width
        });
    }

    hideDropdown() {
        console.log('[FieldPathSearchComponent] Hiding dropdown');
        this.dropdown.style.display = 'none';
        this.activeIndex = -1;
    }

    groupByCategory(fields) {
        const grouped = {};

        fields.forEach(field => {
            const category = field.category || 'Other';
            if (!grouped[category]) {
                grouped[category] = [];
            }
            grouped[category].push(field);
        });

        return grouped;
    }

    highlightMatch(text, query) {
        if (!query) return this.escapeHtml(text);

        const lowerText = this.options.caseSensitive ? text : text.toLowerCase();
        const lowerQuery = this.options.caseSensitive ? query : query.toLowerCase();
        const index = lowerText.indexOf(lowerQuery);

        if (index === -1) return this.escapeHtml(text);

        const before = text.substring(0, index);
        const match = text.substring(index, index + query.length);
        const after = text.substring(index + query.length);

        return `${this.escapeHtml(before)}<mark>${this.escapeHtml(match)}</mark>${this.escapeHtml(after)}`;
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    getCategoryColor(category) {
        const colors = {
            'Patient': '#3b82f6',       // Blue
            'Visit': '#8b5cf6',         // Purple
            'Insurance': '#f59e0b',     // Amber
            'Clinical': '#ef4444',      // Red
            'Contact': '#10b981',       // Green
            'Provider': '#06b6d4',      // Cyan
            'Administrative': '#64748b', // Gray
            'Step Outputs': '#22c55e',  // Emerald - for step variables
            'Variables': '#a855f7',     // Purple - for user variables
            'Database': '#f97316',      // Orange - for database enrichment
            'API': '#0ea5e9',           // Sky - for API enrichment
            'Script': '#eab308'         // Yellow - for script enrichment
        };
        return colors[category] || '#9ca3af';
    }

    getCategoryIcon(category) {
        const icons = {
            'Patient': '👤',
            'Visit': '🏥',
            'Insurance': '💳',
            'Clinical': '⚕️',
            'Contact': '📞',
            'Provider': '👨‍⚕️',
            'Administrative': '📋',
            'Step Outputs': '📦',    // Package icon for step outputs
            'Variables': '📝',       // Memo icon for variables
            'Database': '🗄️',        // File cabinet for DB
            'API': '🌐',             // Globe for API
            'Script': '⚡'           // Lightning for scripts
        };
        return icons[category] || '📄';
    }

    addRecentPath(path) {
        // Remove if already exists
        this.recentPaths = this.recentPaths.filter(p => p !== path);

        // Add to beginning
        this.recentPaths.unshift(path);

        // Keep only last 10
        this.recentPaths = this.recentPaths.slice(0, 10);

        // Save to localStorage
        this.saveRecentPaths();
    }

    loadRecentPaths() {
        try {
            const saved = localStorage.getItem('fieldPathSearch_recent');
            return saved ? JSON.parse(saved) : [];
        } catch (e) {
            return [];
        }
    }

    saveRecentPaths() {
        try {
            localStorage.setItem('fieldPathSearch_recent', JSON.stringify(this.recentPaths));
        } catch (e) {
            console.warn('Failed to save recent paths to localStorage');
        }
    }

    /**
     * Get all available fields for search
     * Combines: HL7 fields + step output variables + additional custom fields
     */
    getAllFields() {
        let fields = [];

        // 1. Add predefined HL7 fields (if enabled)
        if (this.options.includeHL7Fields) {
            fields = fields.concat(this.getHL7Fields());
        }

        // 2. Add step output variables (if callback provided)
        if (this.options.getStepVariables) {
            const stepVars = this.options.getStepVariables();
            if (Array.isArray(stepVars) && stepVars.length > 0) {
                fields = fields.concat(stepVars.map(v => ({
                    ...v,
                    category: v.category || 'Step Outputs'
                })));
            }
        }

        // 3. Add additional fields passed via options
        if (this.options.additionalFields && this.options.additionalFields.length > 0) {
            fields = fields.concat(this.options.additionalFields);
        }

        return fields;
    }

    /**
     * Alias for backward compatibility
     */
    getCommonFields() {
        return this.getAllFields();
    }

    /**
     * Get predefined HL7 fields
     */
    getHL7Fields() {
        return [
            // Patient Identification (PID) - Atomic and Composite fields
            { name: 'Patient MRN (ID Only)', path: 'PID.3.1', description: 'MRN ID Number only (PID-3.1) - atomic, for database lookups', category: 'Patient' },
            { name: 'Patient MRN (Full)', path: 'PID.3', description: 'Full MRN with authority (PID-3) - composite', category: 'Patient' },
            { name: 'MRN Assigning Authority', path: 'PID.3.4', description: 'Assigning Authority (PID-3.4) - atomic', category: 'Patient' },
            { name: 'Patient ID', path: 'PID.2', description: 'Patient ID (PID-2)', category: 'Patient' },
            { name: 'Patient First Name', path: 'PID.5.2', description: 'Given Name (PID-5.2) - atomic', category: 'Patient' },
            { name: 'Patient Last Name', path: 'PID.5.1', description: 'Family Name (PID-5.1) - atomic', category: 'Patient' },
            { name: 'Patient Middle Name', path: 'PID.5.3', description: 'Second Name/Middle (PID-5.3) - atomic', category: 'Patient' },
            { name: 'Patient Name Suffix', path: 'PID.5.4', description: 'Suffix e.g., Jr, Sr (PID-5.4) - atomic', category: 'Patient' },
            { name: 'Patient Name Prefix', path: 'PID.5.5', description: 'Prefix e.g., Dr, Mr (PID-5.5) - atomic', category: 'Patient' },
            { name: 'Patient Full Name', path: 'PID.5', description: 'Full Name (PID-5) - composite', category: 'Patient' },
            { name: 'Date of Birth', path: 'PID.7', description: 'Birth Date (PID-7) - atomic', category: 'Patient' },
            { name: 'Gender', path: 'PID.8', description: 'Administrative Sex M/F/U (PID-8) - atomic', category: 'Patient' },
            { name: 'Race', path: 'PID.10', description: 'Race (PID-10)', category: 'Patient' },
            { name: 'Marital Status', path: 'PID.16', description: 'Marital Status (PID-16)', category: 'Patient' },
            { name: 'SSN', path: 'PID.19', description: 'Social Security Number (PID-19) - atomic', category: 'Patient' },
            { name: 'Account Number', path: 'PID.18', description: 'Patient Account Number (PID-18)', category: 'Patient' },

            // Contact Information
            { name: 'Phone Number', path: 'PID.13', description: 'Phone Number Home (PID-13)', category: 'Contact' },
            { name: 'Email', path: 'PID.13.4', description: 'Email Address (PID-13.4)', category: 'Contact' },
            { name: 'Address', path: 'PID.11', description: 'Patient Address (PID-11)', category: 'Contact' },
            { name: 'Street Address', path: 'PID.11.1', description: 'Street (PID-11.1)', category: 'Contact' },
            { name: 'City', path: 'PID.11.3', description: 'City (PID-11.3)', category: 'Contact' },
            { name: 'State', path: 'PID.11.4', description: 'State (PID-11.4)', category: 'Contact' },
            { name: 'Zip Code', path: 'PID.11.5', description: 'Postal Code (PID-11.5)', category: 'Contact' },

            // Visit Information (PV1)
            { name: 'Visit Number', path: 'PV1.19', description: 'Visit Number (PV1-19)', category: 'Visit' },
            { name: 'Patient Class', path: 'PV1.2', description: 'Inpatient/Outpatient (PV1-2)', category: 'Visit' },
            { name: 'Admission Date', path: 'PV1.44', description: 'Admit Date/Time (PV1-44)', category: 'Visit' },
            { name: 'Discharge Date', path: 'PV1.45', description: 'Discharge Date/Time (PV1-45)', category: 'Visit' },
            { name: 'Patient Location', path: 'PV1.3', description: 'Assigned Patient Location (PV1-3)', category: 'Visit' },
            { name: 'Room Number', path: 'PV1.3.2', description: 'Room (PV1-3.2)', category: 'Visit' },
            { name: 'Bed Number', path: 'PV1.3.3', description: 'Bed (PV1-3.3)', category: 'Visit' },
            { name: 'Hospital Service', path: 'PV1.10', description: 'Hospital Service (PV1-10)', category: 'Visit' },

            // Provider Information
            { name: 'Attending Doctor', path: 'PV1.7', description: 'Attending Doctor (PV1-7)', category: 'Provider' },
            { name: 'Referring Doctor', path: 'PV1.8', description: 'Referring Doctor (PV1-8)', category: 'Provider' },
            { name: 'Admitting Doctor', path: 'PV1.17', description: 'Admitting Doctor (PV1-17)', category: 'Provider' },

            // Insurance (IN1)
            { name: 'Insurance Plan', path: 'IN1.2', description: 'Insurance Plan ID (IN1-2)', category: 'Insurance' },
            { name: 'Insurance Company', path: 'IN1.3', description: 'Insurance Company ID (IN1-3)', category: 'Insurance' },
            { name: 'Insurance Group Number', path: 'IN1.8', description: 'Group Number (IN1-8)', category: 'Insurance' },
            { name: 'Policy Number', path: 'IN1.36', description: 'Policy Number (IN1-36)', category: 'Insurance' },

            // Message Control (MSH)
            { name: 'Message Control ID', path: 'MSH.10', description: 'Message Control ID (MSH-10)', category: 'Administrative' },
            { name: 'Message Type (Full)', path: 'MSH.9', description: 'Full Message Type ADT^A01 (MSH-9)', category: 'Administrative' },
            { name: 'Message Type Code', path: 'MSH.9.1', description: 'Message Code e.g., ADT (MSH-9.1) - atomic', category: 'Administrative' },
            { name: 'Trigger Event', path: 'MSH.9.2', description: 'Trigger Event e.g., A01 (MSH-9.2) - atomic', category: 'Administrative' },
            { name: 'Message Structure', path: 'MSH.9.3', description: 'Message Structure (MSH-9.3) - atomic', category: 'Administrative' },
            { name: 'Sending Application', path: 'MSH.3', description: 'Sending Application (MSH-3)', category: 'Administrative' },
            { name: 'Sending App Name', path: 'MSH.3.1', description: 'Sending Application Name (MSH-3.1) - atomic', category: 'Administrative' },
            { name: 'Sending Facility', path: 'MSH.4', description: 'Sending Facility (MSH-4)', category: 'Administrative' },
            { name: 'Sending Facility Name', path: 'MSH.4.1', description: 'Sending Facility Name (MSH-4.1) - atomic', category: 'Administrative' },
            { name: 'Receiving Application', path: 'MSH.5', description: 'Receiving Application (MSH-5)', category: 'Administrative' },
            { name: 'Receiving Facility', path: 'MSH.6', description: 'Receiving Facility (MSH-6)', category: 'Administrative' },
            { name: 'Message DateTime', path: 'MSH.7', description: 'Message Date/Time (MSH-7)', category: 'Administrative' },
            { name: 'HL7 Version', path: 'MSH.12', description: 'HL7 Version ID (MSH-12)', category: 'Administrative' },

            // Clinical (OBX)
            { name: 'Observation Value', path: 'OBX.5', description: 'Observation Value (OBX-5)', category: 'Clinical' },
            { name: 'Observation ID', path: 'OBX.3', description: 'Observation Identifier (OBX-3)', category: 'Clinical' },
            { name: 'Observation Units', path: 'OBX.6', description: 'Units (OBX-6)', category: 'Clinical' }
        ];
    }

    injectStyles() {
        // Only inject once
        if (document.getElementById('field-path-search-styles')) {
            return;
        }

        const style = document.createElement('style');
        style.id = 'field-path-search-styles';
        style.textContent = `
            /* FieldPathSearchComponent Styles */
            .field-path-search-dropdown {
                position: fixed;
                background: white;
                border: 1px solid #ddd;
                border-radius: 6px;
                box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
                max-height: 400px;
                overflow-y: auto;
                z-index: 10000;
            }

            .field-path-category {
                padding: 8px 12px;
                background: #f8f9fa;
                font-size: 11px;
                font-weight: 600;
                color: #666;
                text-transform: uppercase;
                letter-spacing: 0.5px;
                border-bottom: 1px solid #e9ecef;
            }

            .field-path-item {
                display: flex;
                align-items: flex-start;
                gap: 10px;
                padding: 10px 12px;
                border-bottom: 1px solid #f0f0f0;
                cursor: pointer;
                transition: background 0.15s;
            }

            .field-path-item:last-child {
                border-bottom: none;
            }

            .field-path-item:hover {
                background: #f5f8fb;
            }

            .field-path-item.active {
                background: #e8f4ff;
            }

            .field-path-item.custom-path {
                background: #fffbf0;
            }

            .field-path-item.custom-path:hover {
                background: #fff9e6;
            }

            .field-path-icon {
                width: 28px;
                height: 28px;
                display: flex;
                align-items: center;
                justify-content: center;
                border-radius: 6px;
                font-size: 14px;
                flex-shrink: 0;
            }

            .field-path-content {
                flex: 1;
                min-width: 0;
            }

            .field-path-name {
                font-weight: 600;
                font-size: 13px;
                color: #333;
                margin-bottom: 2px;
            }

            .field-path-path {
                font-family: 'Courier New', monospace;
                font-size: 11px;
                color: #4a90e2;
                background: #f0f7ff;
                padding: 2px 6px;
                border-radius: 3px;
                display: inline-block;
                margin-bottom: 3px;
                word-break: break-all;
            }

            .field-path-desc {
                font-size: 11px;
                color: #666;
            }

            .field-path-highlight {
                background: #ffeb3b;
                font-weight: 600;
                padding: 0 2px;
                border-radius: 2px;
            }

            /* Browse-mode hint row */
            .field-path-hint {
                padding: 8px 12px;
                font-size: 11px;
                color: #888;
                text-align: center;
                border-top: 1px solid #f0f0f0;
            }
            .field-path-hint code {
                background: #f0f7ff;
                color: #4a90e2;
                padding: 1px 4px;
                border-radius: 3px;
                font-size: 11px;
            }

            /* Scrollbar */
            .field-path-search-dropdown::-webkit-scrollbar {
                width: 8px;
            }

            .field-path-search-dropdown::-webkit-scrollbar-track {
                background: #f1f1f1;
                border-radius: 4px;
            }

            .field-path-search-dropdown::-webkit-scrollbar-thumb {
                background: #ccc;
                border-radius: 4px;
            }

            .field-path-search-dropdown::-webkit-scrollbar-thumb:hover {
                background: #999;
            }
        `;
        document.head.appendChild(style);
    }

    destroy() {
        if (this.dropdown && this.dropdown.parentNode) {
            this.dropdown.parentNode.removeChild(this.dropdown);
        }
    }
}
