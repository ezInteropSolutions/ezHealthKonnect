/**
 * StaticFieldDropdown - MINIMAL VERSION v2.0
 * Zero memory leaks, comprehensive logging, debounced search
 */

class StaticFieldDropdown {
    constructor(container, options = {}) {
        console.log('[StaticFieldDropdown v2.0] Constructor called');
        this.container = container;
        this.options = {
            initialValue: options.initialValue || '',
            onChange: options.onChange || (() => {})
        };

        // Static list of common HL7 fields (NO API call needed!)
        this.allFields = this.getCommonFields();
        this.filteredFields = [...this.allFields];

        // Debounce timer to prevent crash while typing
        this.filterDebounceTimer = null;

        // Bind event handlers ONCE (critical for cleanup)
        this.handleSearchInput = this.handleSearchInput.bind(this);
        this.handleSearchChange = this.handleSearchChange.bind(this);
        this.handleDropdownChange = this.handleDropdownChange.bind(this);
        this.handleDropdownDblClick = this.handleDropdownDblClick.bind(this);

        this.render();
        this.attachEventListeners();
        console.log('[StaticFieldDropdown] Initialized with', this.allFields.length, 'fields');
    }

    /**
     * Static list of most common HL7 v2.x fields
     */
    getCommonFields() {
        return [
            // MSH - Message Header
            { path: 'enhancedSegments.MSH.fields[0].value', label: 'MSH.3 - Sending Application' },
            { path: 'enhancedSegments.MSH.fields[1].value', label: 'MSH.9 - Message Type' },
            { path: 'enhancedSegments.MSH.fields[2].value', label: 'MSH.10 - Message Control ID' },

            // PID - Patient Identification
            { path: 'enhancedSegments.PID.fields[0].value', label: 'PID.3 - Patient ID' },
            { path: 'enhancedSegments.PID.fields[1].value', label: 'PID.5 - Patient Name' },
            { path: 'enhancedSegments.PID.fields[2].value', label: 'PID.7 - Date of Birth' },
            { path: 'enhancedSegments.PID.fields[3].value', label: 'PID.8 - Administrative Sex' },
            { path: 'enhancedSegments.PID.fields[4].value', label: 'PID.11 - Patient Address' },
            { path: 'enhancedSegments.PID.fields[5].value', label: 'PID.13 - Phone Number - Home' },
            { path: 'enhancedSegments.PID.fields[7].value', label: 'PID.18 - Patient Account Number' },

            // PV1 - Patient Visit
            { path: 'enhancedSegments.PV1.fields[0].value', label: 'PV1.2 - Patient Class' },
            { path: 'enhancedSegments.PV1.fields[1].value', label: 'PV1.3 - Assigned Patient Location' },
            { path: 'enhancedSegments.PV1.fields[6].value', label: 'PV1.19 - Visit Number' },
            { path: 'enhancedSegments.PV1.fields[9].value', label: 'PV1.44 - Admit Date/Time' },

            // OBR - Observation Request
            { path: 'enhancedSegments.OBR.fields[0].value', label: 'OBR.2 - Placer Order Number' },
            { path: 'enhancedSegments.OBR.fields[3].value', label: 'OBR.4 - Universal Service ID' },
            { path: 'enhancedSegments.OBR.fields[5].value', label: 'OBR.7 - Observation Date/Time' },

            // OBX - Observation/Result
            { path: 'enhancedSegments.OBX.fields[0].value', label: 'OBX.2 - Value Type' },
            { path: 'enhancedSegments.OBX.fields[2].value', label: 'OBX.3 - Observation Identifier' },
            { path: 'enhancedSegments.OBX.fields[4].value', label: 'OBX.5 - Observation Value' },
            { path: 'enhancedSegments.OBX.fields[5].value', label: 'OBX.6 - Units' },
            { path: 'enhancedSegments.OBX.fields[10].value', label: 'OBX.11 - Observation Result Status' },

            // DG1 - Diagnosis
            { path: 'enhancedSegments.DG1.fields[2].value', label: 'DG1.3 - Diagnosis Code' },
            { path: 'enhancedSegments.DG1.fields[3].value', label: 'DG1.4 - Diagnosis Description' },

            // AL1 - Allergy Information
            { path: 'enhancedSegments.AL1.fields[2].value', label: 'AL1.3 - Allergen Code' },
            { path: 'enhancedSegments.AL1.fields[4].value', label: 'AL1.5 - Allergy Reaction Code' }
        ];
    }

    render() {
        console.log('[StaticFieldDropdown] render() called');
        this.container.innerHTML = `
            <div class="static-dropdown-wrapper">
                <input
                    type="text"
                    class="dropdown-search form-control"
                    placeholder="Search or type custom path..."
                    value="${this.options.initialValue}"
                />
                <select class="field-dropdown form-control" size="8" style="display: block; width: 100%;">
                    ${this.renderOptions()}
                </select>
            </div>
        `;

        this.searchInput = this.container.querySelector('.dropdown-search');
        this.dropdown = this.container.querySelector('.field-dropdown');
        console.log('[StaticFieldDropdown] DOM elements created');
    }

    renderOptions() {
        return this.filteredFields
            .map(field => `<option value="${field.path}">${field.label}</option>`)
            .join('');
    }

    attachEventListeners() {
        console.log('[StaticFieldDropdown] attachEventListeners() called');

        // DEBOUNCED search input - prevents crash while typing
        this.searchInput.addEventListener('input', this.handleSearchInput);

        // Manual entry allowed
        this.searchInput.addEventListener('change', this.handleSearchChange);

        // Dropdown selection
        this.dropdown.addEventListener('change', this.handleDropdownChange);

        // Double-click to select
        this.dropdown.addEventListener('dblclick', this.handleDropdownDblClick);

        console.log('[StaticFieldDropdown] 4 event listeners attached');
    }

    // Bound event handlers (can be removed in destroy())
    handleSearchInput() {
        console.log('[StaticFieldDropdown] Search input - debouncing...');
        // Debounce to prevent excessive filtering
        if (this.filterDebounceTimer) {
            clearTimeout(this.filterDebounceTimer);
        }

        this.filterDebounceTimer = setTimeout(() => {
            console.log('[StaticFieldDropdown] Applying filter');
            this.filterFields();
        }, 200); // 200ms debounce
    }

    handleSearchChange() {
        const value = this.searchInput.value.trim();
        console.log('[StaticFieldDropdown] Search changed:', value);
        this.options.onChange(value);
    }

    handleDropdownChange() {
        const selectedPath = this.dropdown.value;
        console.log('[StaticFieldDropdown] Dropdown selection:', selectedPath);
        this.searchInput.value = selectedPath;
        this.options.onChange(selectedPath);
    }

    handleDropdownDblClick() {
        const selectedPath = this.dropdown.value;
        if (selectedPath) {
            console.log('[StaticFieldDropdown] Double-click:', selectedPath);
            this.searchInput.value = selectedPath;
            this.options.onChange(selectedPath);
        }
    }

    filterFields() {
        const query = this.searchInput.value.toLowerCase();
        console.log('[StaticFieldDropdown] Filtering with query:', query);

        if (!query) {
            this.filteredFields = [...this.allFields];
        } else {
            this.filteredFields = this.allFields.filter(field =>
                field.label.toLowerCase().includes(query) ||
                field.path.toLowerCase().includes(query)
            );
        }

        console.log('[StaticFieldDropdown] Filtered results:', this.filteredFields.length);

        // Update dropdown options
        this.dropdown.innerHTML = this.renderOptions();
    }

    getValue() {
        return this.searchInput.value.trim();
    }

    setValue(value) {
        this.searchInput.value = value;
    }

    destroy() {
        console.log('[StaticFieldDropdown] destroy() called - removing all event listeners');

        // Remove ALL event listeners (CRITICAL!)
        if (this.searchInput) {
            this.searchInput.removeEventListener('input', this.handleSearchInput);
            this.searchInput.removeEventListener('change', this.handleSearchChange);
        }

        if (this.dropdown) {
            this.dropdown.removeEventListener('change', this.handleDropdownChange);
            this.dropdown.removeEventListener('dblclick', this.handleDropdownDblClick);
        }

        // Clear debounce timer
        if (this.filterDebounceTimer) {
            clearTimeout(this.filterDebounceTimer);
        }

        // Null out references
        this.searchInput = null;
        this.dropdown = null;
        this.container = null;
        this.allFields = null;
        this.filteredFields = null;

        console.log('[StaticFieldDropdown] Destroyed - all 4 listeners removed');
    }
}

// Make available globally
if (typeof window !== 'undefined') {
    window.StaticFieldDropdown = StaticFieldDropdown;
}
