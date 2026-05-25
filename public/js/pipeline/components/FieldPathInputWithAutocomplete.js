/**
 * FieldPathInputWithAutocomplete - Field path input with intelligent autocomplete
 * Fetches field data from MongoDB parsed messages (via PostgreSQL sample_parsed_messages)
 * Supports both path-based and description-based search
 */

class FieldPathInputWithAutocomplete {
    constructor(container, options = {}) {
        this.container = container;
        this.options = {
            initialValue: options.initialValue || '',
            onChange: options.onChange || (() => {}),
            searchMode: options.searchMode || 'path' // Global search mode
        };

        // Shared field data (loaded once, shared across all instances)
        this.fields = FieldPathInputWithAutocomplete.sharedFields || [];
        this.filteredFields = [];
        this.selectedIndex = -1;

        // Bind ALL methods to preserve 'this' context (required for removeEventListener)
        this.handleInput = this.handleInput.bind(this);
        this.handleKeydown = this.handleKeydown.bind(this);
        this.handleBlur = this.handleBlur.bind(this);
        this.handleChange = this.handleChange.bind(this);
        this.handleDropdownClick = this.handleDropdownClick.bind(this);

        this.render();
        this.attachEventListeners();

        // Load fields if not already loaded
        if (!FieldPathInputWithAutocomplete.sharedFields) {
            this.loadFields();
        }
    }

    /**
     * Load field data from API (from PostgreSQL sample_parsed_messages, NOT schema files)
     */
    async loadFields() {
        if (FieldPathInputWithAutocomplete.isLoading) return;
        FieldPathInputWithAutocomplete.isLoading = true;

        try {
            console.log('[FieldPathInput] Loading fields from sample_parsed_messages...');
            const response = await fetch('/api/schemas/hl7/fields');

            // Check if endpoint exists
            if (!response.ok) {
                if (response.status === 404) {
                    console.log('[FieldPathInput] Autocomplete API not available - manual field entry enabled');
                    return;
                }
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }

            const data = await response.json();

            if (data.success && data.xpathTree) {
                // Flatten the tree into searchable list
                const fields = this.flattenTree(data.xpathTree);
                FieldPathInputWithAutocomplete.sharedFields = fields;
                this.fields = fields;
                console.log(`[FieldPathInput] Loaded ${fields.length} fields from database`);
            }
        } catch (error) {
            // Only log if it's not a 404 (which we already handled)
            if (error.message && !error.message.includes('404')) {
                console.warn('[FieldPathInput] Could not load autocomplete fields:', error.message);
            }
        } finally {
            FieldPathInputWithAutocomplete.isLoading = false;
        }
    }

    /**
     * Flatten tree structure into searchable array
     * CRITICAL: Use field KEY (e.g., "PID.3", "MSH.9") not complex path
     * Backend expects simple HL7 field keys and handles path resolution automatically
     */
    flattenTree(node, result = []) {
        // Include field-value nodes (e.g., PID.3, PID.5, PID.7)
        // AND string nodes (subfields like PID.5.1, PID.5.2)
        if ((node.type === 'field-value' || node.type === 'string') && node.path) {
            // CRITICAL FIX: Use field KEY from node.name instead of complex path
            // SampleMessageService.buildXPathTree() sets node.name to field.key (PID.3, MSH.9, etc.)
            // The complex path like "enhancedSegments.PID.fields[0].value" should NOT be used
            const fieldKey = node.name || '';

            result.push({
                path: fieldKey, // Use simple HL7 key (PID.3, MSH.9) not complex path
                name: fieldKey,
                description: node.description || '',
                dataType: node.dataType || '',
                displayText: `${fieldKey} - ${node.description || 'No description'}`,
                example: node.example || ''
            });
        }

        if (node.children) {
            node.children.forEach(child => this.flattenTree(child, result));
        }

        return result;
    }

    render() {
        this.container.innerHTML = `
            <div class="field-path-autocomplete-wrapper">
                <input
                    type="text"
                    class="field-path-input form-control"
                    placeholder="${this.getPlaceholder()}"
                    value="${this.options.initialValue}"
                    autocomplete="off"
                />
                <div class="autocomplete-dropdown" style="display: none;"></div>
            </div>
        `;

        this.elements = {
            input: this.container.querySelector('.field-path-input'),
            dropdown: this.container.querySelector('.autocomplete-dropdown')
        };
    }

    getPlaceholder() {
        return this.options.searchMode === 'path'
            ? 'Enter field key (e.g., PID.3, MSH.9, PID.5.1)'
            : 'Search by description (e.g., Date of Birth, Patient Name)';
    }

    attachEventListeners() {
        // Attach all event listeners using bound methods (can be removed later)
        this.elements.input.addEventListener('input', this.handleInput);
        this.elements.input.addEventListener('keydown', this.handleKeydown);
        this.elements.input.addEventListener('blur', this.handleBlur);
        this.elements.input.addEventListener('change', this.handleChange);
        this.elements.dropdown.addEventListener('mousedown', this.handleDropdownClick);
    }

    handleInput(e) {
        const value = e.target ? e.target.value : e; // Support both event and string
        if (!value || value.length < 2) {
            this.hideDropdown();
            return;
        }

        // Filter fields based on search mode
        this.filteredFields = this.filterFields(value);

        if (this.filteredFields.length > 0) {
            this.showDropdown();
        } else {
            this.hideDropdown();
        }
    }

    handleKeydown(e) {
        if (e.key === 'ArrowDown') {
            e.preventDefault();
            this.selectNext();
        } else if (e.key === 'ArrowUp') {
            e.preventDefault();
            this.selectPrevious();
        } else if (e.key === 'Enter') {
            e.preventDefault();
            this.selectCurrent();
        } else if (e.key === 'Escape') {
            this.hideDropdown();
        }
    }

    handleBlur() {
        setTimeout(() => this.hideDropdown(), 200);
    }

    handleChange() {
        this.options.onChange(this.elements.input.value.trim());
    }

    /**
     * Handle dropdown clicks using event delegation (prevents memory leaks)
     */
    handleDropdownClick(e) {
        e.preventDefault(); // Prevent input blur

        // Find the clicked item
        const item = e.target.closest('.autocomplete-item');
        if (item) {
            const index = parseInt(item.dataset.index, 10);
            if (!isNaN(index)) {
                this.selectItem(index);
            }
        }
    }

    filterFields(query) {
        const lowerQuery = query.toLowerCase();

        return this.fields
            .filter(field => {
                // Search in ALL fields: path, description, and name
                return (
                    field.path.toLowerCase().includes(lowerQuery) ||
                    (field.description && field.description.toLowerCase().includes(lowerQuery)) ||
                    (field.name && field.name.toLowerCase().includes(lowerQuery))
                );
            })
            .slice(0, 10); // Limit to 10 results
    }

    showDropdown() {
        const items = this.filteredFields.map((field, index) => {
            const isSelected = index === this.selectedIndex;
            return `
                <div class="autocomplete-item ${isSelected ? 'selected' : ''}" data-index="${index}">
                    <div class="field-name">${field.name}</div>
                    <div class="field-description">${field.description || 'No description'}</div>
                    <div class="field-path">${field.path}</div>
                </div>
            `;
        }).join('');

        this.elements.dropdown.innerHTML = items;
        this.elements.dropdown.style.display = 'block';

        // NO event listeners attached here - using event delegation on parent!
    }

    hideDropdown() {
        this.elements.dropdown.style.display = 'none';
        this.selectedIndex = -1;
    }

    selectNext() {
        if (this.filteredFields.length === 0) return;
        this.selectedIndex = (this.selectedIndex + 1) % this.filteredFields.length;
        this.showDropdown();
    }

    selectPrevious() {
        if (this.filteredFields.length === 0) return;
        this.selectedIndex = this.selectedIndex <= 0 ? this.filteredFields.length - 1 : this.selectedIndex - 1;
        this.showDropdown();
    }

    selectCurrent() {
        if (this.selectedIndex >= 0 && this.selectedIndex < this.filteredFields.length) {
            this.selectItem(this.selectedIndex);
        }
    }

    selectItem(index) {
        const field = this.filteredFields[index];
        if (field) {
            console.log('[FieldPathInput] Selected:', field.path);
            this.elements.input.value = field.path;
            this.elements.input.focus(); // Keep focus on input
            this.options.onChange(field.path, field);
            this.hideDropdown();
        }
    }

    setSearchMode(mode) {
        this.options.searchMode = mode;
        this.elements.input.placeholder = this.getPlaceholder();
        this.hideDropdown();
    }

    getValue() {
        return this.elements.input.value.trim();
    }

    setValue(value) {
        this.elements.input.value = value;
    }

    destroy() {
        // Remove ALL event listeners (CRITICAL for preventing memory leaks)
        if (this.elements) {
            if (this.elements.input) {
                this.elements.input.removeEventListener('input', this.handleInput);
                this.elements.input.removeEventListener('keydown', this.handleKeydown);
                this.elements.input.removeEventListener('blur', this.handleBlur);
                this.elements.input.removeEventListener('change', this.handleChange);
            }
            if (this.elements.dropdown) {
                this.elements.dropdown.removeEventListener('mousedown', this.handleDropdownClick);
            }
        }

        // Null out all references to aid garbage collection
        this.elements = null;
        this.container = null;
        this.fields = null;
        this.filteredFields = null;
        this.options = null;

        console.log('[FieldPathInput] Instance destroyed - all 5 event listeners removed');
    }
}

// Shared static fields (loaded once, reused by all instances)
FieldPathInputWithAutocomplete.sharedFields = null;
FieldPathInputWithAutocomplete.isLoading = false;

// Make available globally
if (typeof window !== 'undefined') {
    window.FieldPathInputWithAutocomplete = FieldPathInputWithAutocomplete;
}
