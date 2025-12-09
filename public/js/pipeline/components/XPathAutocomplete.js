/**
 * XPathAutocomplete Component
 * Smart field path selector with IntelliSense for HL7 and FHIR schemas
 *
 * Usage:
 * const autocomplete = new XPathAutocomplete(container, {
 *     format: 'hl7v2',
 *     version: 'v2.5',
 *     messageType: 'ADT_A01',
 *     placeholder: 'Select field path...',
 *     onChange: (path) => { console.log('Selected:', path); }
 * });
 */

class XPathAutocomplete {
    // Static cache to prevent loading same schema multiple times
    static schemaCache = new Map();
    static loadingPromises = new Map();

    constructor(container, options = {}) {
        this.container = container;
        this.options = {
            format: options.format || 'hl7v2',
            version: options.version || 'v2.5',
            messageType: options.messageType || '',
            resourceType: options.resourceType || '',
            placeholder: options.placeholder || 'Type to search field paths...',
            onChange: options.onChange || (() => {}),
            allowCustomPath: options.allowCustomPath !== false,
            initialValue: options.initialValue || ''
        };

        this.schema = null;
        this.flattenedPaths = [];
        this.filteredPaths = [];
        this.selectedIndex = -1;
        this.isLoading = false;

        this.render();
        this.attachEventListeners();

        // Load schema if format and type provided
        if (this.canLoadSchema()) {
            this.loadSchema();
        }
    }

    canLoadSchema() {
        if (this.options.format === 'hl7v2') {
            return this.options.version && this.options.messageType;
        } else if (this.options.format === 'fhir') {
            return this.options.version && this.options.resourceType;
        }
        return false;
    }

    render() {
        console.log('[XPathAutocomplete] Rendering into container:', this.container);

        this.container.innerHTML = `
            <div class="xpath-autocomplete">
                <div class="xpath-input-wrapper">
                    <input
                        type="text"
                        class="xpath-input form-control"
                        placeholder="${this.options.placeholder}"
                        value="${this.options.initialValue}"
                        autocomplete="off"
                    />
                    <div class="xpath-loading" style="display: none;">
                        <i class="fas fa-spinner fa-spin"></i>
                    </div>
                </div>
                <div class="xpath-dropdown" style="display: none;">
                    <div class="xpath-dropdown-list"></div>
                </div>
                <div class="xpath-error" style="display: none; color: red; font-size: 0.875rem; margin-top: 4px;"></div>
            </div>
        `;

        this.elements = {
            input: this.container.querySelector('.xpath-input'),
            dropdown: this.container.querySelector('.xpath-dropdown'),
            dropdownList: this.container.querySelector('.xpath-dropdown-list'),
            loading: this.container.querySelector('.xpath-loading'),
            error: this.container.querySelector('.xpath-error')
        };

        console.log('[XPathAutocomplete] Elements found:', {
            input: !!this.elements.input,
            dropdown: !!this.elements.dropdown,
            dropdownList: !!this.elements.dropdownList
        });
    }

    attachEventListeners() {
        // Store bound handlers for cleanup
        this.handlers = {
            input: (e) => this.handleInput(e),
            focus: () => this.handleFocus(),
            blur: () => this.handleBlur(),
            keydown: (e) => this.handleKeydown(e),
            documentClick: (e) => {
                if (!this.container.contains(e.target)) {
                    this.hideDropdown();
                }
            }
        };

        // Input events
        this.elements.input.addEventListener('input', this.handlers.input);
        this.elements.input.addEventListener('focus', this.handlers.focus);
        this.elements.input.addEventListener('blur', this.handlers.blur);
        this.elements.input.addEventListener('keydown', this.handlers.keydown);

        // Click outside to close
        document.addEventListener('click', this.handlers.documentClick);
    }

    /**
     * Destroy the autocomplete instance and cleanup
     */
    destroy() {
        // Remove event listeners
        if (this.handlers) {
            this.elements.input?.removeEventListener('input', this.handlers.input);
            this.elements.input?.removeEventListener('focus', this.handlers.focus);
            this.elements.input?.removeEventListener('blur', this.handlers.blur);
            this.elements.input?.removeEventListener('keydown', this.handlers.keydown);
            document.removeEventListener('click', this.handlers.documentClick);
        }

        // Clear references
        this.schema = null;
        this.flattenedPaths = [];
        this.filteredPaths = [];
        this.handlers = null;
        this.elements = null;
        this.container = null;

        console.log('[XPathAutocomplete] Instance destroyed and cleaned up');
    }

    async loadSchema() {
        this.isLoading = true;
        this.showLoading();
        this.hideError();

        try {
            // Use universal field paths endpoint (format-agnostic)
            let url;
            if (this.options.format === 'hl7v2') {
                url = `/api/schemas/hl7/fields`;  // Universal endpoint - all HL7 fields
            } else if (this.options.format === 'fhir') {
                url = `/api/schemas/fhir/fields`;  // Universal endpoint - all FHIR fields
            } else if (this.options.format === 'cda' || this.options.format === 'ccd') {
                url = `/api/schemas/cda/fields`;   // Universal endpoint - all CDA/CCD fields
            } else {
                throw new Error(`Unsupported format: ${this.options.format}`);
            }

            const cacheKey = `${this.options.format}_${url}`;

            // Check cache first
            if (XPathAutocomplete.schemaCache.has(cacheKey)) {
                console.log(`📦 Using cached schema for: ${cacheKey}`);
                const cached = XPathAutocomplete.schemaCache.get(cacheKey);
                this.schema = cached.schema;
                this.flattenedPaths = cached.flattenedPaths;
                console.log(`✅ Loaded ${this.flattenedPaths.length} paths from cache`);
                return;
            }

            // Check if already loading
            if (XPathAutocomplete.loadingPromises.has(cacheKey)) {
                console.log(`⏳ Waiting for existing schema load: ${cacheKey}`);
                const cached = await XPathAutocomplete.loadingPromises.get(cacheKey);
                this.schema = cached.schema;
                this.flattenedPaths = cached.flattenedPaths;
                console.log(`✅ Loaded ${this.flattenedPaths.length} paths from pending load`);
                return;
            }

            // Create loading promise
            const loadPromise = (async () => {
                console.log(`📡 Loading universal field paths from: ${url}`);

                const response = await fetch(url);
                if (!response.ok) {
                    throw new Error(`Failed to load schema: ${response.statusText}`);
                }

                const data = await response.json();
                const schema = data.xpathTree || data.schema;

                // Flatten schema tree for autocomplete
                // IMPORTANT: Initialize visited Set ONCE for the entire traversal
                const visited = new Set();
                const flattenedPaths = this.flattenSchemaTree(schema, [], visited, 0);

                console.log(`✅ Loaded ${flattenedPaths.length} universal field paths`);
                console.log(`📊 Visited nodes: ${visited.size}, Max depth: ${this.maxDepthReached || 0}`);

                const cached = { schema, flattenedPaths };
                XPathAutocomplete.schemaCache.set(cacheKey, cached);
                XPathAutocomplete.loadingPromises.delete(cacheKey);

                return cached;
            })();

            XPathAutocomplete.loadingPromises.set(cacheKey, loadPromise);

            const cached = await loadPromise;
            this.schema = cached.schema;
            this.flattenedPaths = cached.flattenedPaths;

        } catch (error) {
            console.error('Failed to load schema:', error);
            this.showError(`Failed to load schema: ${error.message}`);
        } finally {
            this.isLoading = false;
            this.hideLoading();
        }
    }

    flattenSchemaTree(node, paths, visited, depth) {
        // Prevent infinite recursion
        if (!node || depth > 20) {
            if (depth > 20) {
                console.warn('[XPathAutocomplete] Max recursion depth reached at depth:', depth);
                this.maxDepthReached = Math.max(this.maxDepthReached || 0, depth);
            }
            return paths;
        }

        // Track max depth for debugging
        this.maxDepthReached = Math.max(this.maxDepthReached || 0, depth);

        // Prevent circular references by tracking visited paths
        const nodeId = node.path || `node_${depth}_${paths.length}`;
        if (node.path && visited.has(nodeId)) {
            console.warn('[XPathAutocomplete] Duplicate path detected at depth', depth, ':', nodeId);
            return paths;
        }
        if (node.path) {
            visited.add(nodeId);
        }

        // Add current node if it has a path
        if (node.path) {
            paths.push({
                path: node.path,
                name: node.name || node.path,
                description: node.description || '',
                dataType: node.dataType || '',
                cardinality: node.cardinality || '',
                level: (node.path.match(/\./g) || []).length
            });
        }

        // Recursively process children
        if (node.children && Array.isArray(node.children)) {
            for (const child of node.children) {
                this.flattenSchemaTree(child, paths, visited, depth + 1);
            }
        }

        return paths;
    }

    handleInput(e) {
        const query = e.target.value.trim();

        if (!query) {
            this.hideDropdown();
            return;
        }

        // Filter paths based on query
        this.filteredPaths = this.searchPaths(query);

        if (this.filteredPaths.length > 0) {
            this.renderDropdown();
            this.showDropdown();
        } else {
            this.hideDropdown();
        }

        this.selectedIndex = -1;
    }

    searchPaths(query) {
        if (!this.flattenedPaths || this.flattenedPaths.length === 0) {
            return [];
        }

        const lowerQuery = query.toLowerCase();

        // Score and filter paths
        const scored = this.flattenedPaths
            .map(item => {
                const pathLower = item.path.toLowerCase();
                const nameLower = item.name.toLowerCase();
                const descLower = (item.description || '').toLowerCase();

                let score = 0;

                // Exact match (highest priority)
                if (pathLower === lowerQuery) score += 100;
                if (nameLower === lowerQuery) score += 95;
                if (descLower === lowerQuery) score += 90;

                // Starts with (high priority for natural typing)
                if (pathLower.startsWith(lowerQuery)) score += 50;
                if (nameLower.startsWith(lowerQuery)) score += 45;
                if (descLower.startsWith(lowerQuery)) score += 40;

                // Contains (boost description matching significantly)
                if (pathLower.includes(lowerQuery)) score += 20;
                if (nameLower.includes(lowerQuery)) score += 25;
                if (descLower.includes(lowerQuery)) score += 30; // Increased from 10 to 30

                // Bonus for shorter paths (more specific)
                score += Math.max(0, 10 - item.level);

                return { item, score };
            })
            .filter(({ score }) => score > 0)
            .sort((a, b) => b.score - a.score)
            .slice(0, 50) // Limit to top 50 results
            .map(({ item }) => item);

        return scored;
    }

    renderDropdown() {
        if (this.filteredPaths.length === 0) {
            this.elements.dropdownList.innerHTML = '<div class="xpath-dropdown-empty">No matching fields found</div>';
            return;
        }

        const html = this.filteredPaths.map((item, index) => {
            // Extract field key from name (e.g., "PID.5" from "PID.5 - Patient Name")
            const fieldKey = item.name && item.name.includes('.') ? item.name.split(' ')[0] : '';

            return `
                <div class="xpath-dropdown-item" data-index="${index}" data-path="${this.escapeHtml(item.path)}">
                    <div class="xpath-item-header">
                        ${fieldKey ? `<span class="xpath-item-key">${this.highlightMatch(fieldKey)}</span>` : ''}
                        ${item.description ? `<span class="xpath-item-name">${this.highlightMatch(item.description)}</span>` : `<span class="xpath-item-name">${this.highlightMatch(item.name)}</span>`}
                    </div>
                    <div class="xpath-item-details">
                        <span class="xpath-item-path-small">${this.escapeHtml(item.path)}</span>
                        ${item.dataType ? `<span class="xpath-item-type">${this.escapeHtml(item.dataType)}</span>` : ''}
                        ${item.cardinality ? `<span class="xpath-item-cardinality">${this.escapeHtml(item.cardinality)}</span>` : ''}
                    </div>
                </div>
            `;
        }).join('');

        this.elements.dropdownList.innerHTML = html;

        // Attach click handlers
        this.elements.dropdownList.querySelectorAll('.xpath-dropdown-item').forEach(item => {
            item.addEventListener('mousedown', (e) => {
                e.preventDefault(); // Prevent blur event
                const path = item.dataset.path;
                this.selectPath(path);
            });

            item.addEventListener('mouseenter', (e) => {
                const index = parseInt(item.dataset.index);
                this.highlightItem(index);
            });
        });
    }

    highlightMatch(text) {
        const query = this.elements.input.value.trim();
        if (!query) return this.escapeHtml(text);

        const regex = new RegExp(`(${this.escapeRegex(query)})`, 'gi');
        return this.escapeHtml(text).replace(regex, '<strong>$1</strong>');
    }

    highlightItem(index) {
        this.selectedIndex = index;

        this.elements.dropdownList.querySelectorAll('.xpath-dropdown-item').forEach((item, i) => {
            if (i === index) {
                item.classList.add('active');
            } else {
                item.classList.remove('active');
            }
        });
    }

    handleFocus() {
        const query = this.elements.input.value.trim();
        if (query && this.filteredPaths.length > 0) {
            this.showDropdown();
        }
    }

    handleBlur() {
        // Delay to allow click events to fire
        setTimeout(() => {
            this.hideDropdown();
        }, 200);
    }

    handleKeydown(e) {
        if (!this.isDropdownVisible()) {
            return;
        }

        switch (e.key) {
            case 'ArrowDown':
                e.preventDefault();
                this.selectedIndex = Math.min(this.selectedIndex + 1, this.filteredPaths.length - 1);
                this.highlightItem(this.selectedIndex);
                this.scrollToSelected();
                break;

            case 'ArrowUp':
                e.preventDefault();
                this.selectedIndex = Math.max(this.selectedIndex - 1, 0);
                this.highlightItem(this.selectedIndex);
                this.scrollToSelected();
                break;

            case 'Enter':
                e.preventDefault();
                if (this.selectedIndex >= 0 && this.selectedIndex < this.filteredPaths.length) {
                    const path = this.filteredPaths[this.selectedIndex].path;
                    this.selectPath(path);
                }
                break;

            case 'Escape':
                e.preventDefault();
                this.hideDropdown();
                break;
        }
    }

    scrollToSelected() {
        const activeItem = this.elements.dropdownList.querySelector('.xpath-dropdown-item.active');
        if (activeItem) {
            activeItem.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
        }
    }

    selectPath(path) {
        this.elements.input.value = path;
        this.hideDropdown();
        this.options.onChange(path);
    }

    showDropdown() {
        this.elements.dropdown.style.display = 'block';
    }

    hideDropdown() {
        this.elements.dropdown.style.display = 'none';
        this.selectedIndex = -1;
    }

    isDropdownVisible() {
        return this.elements.dropdown.style.display === 'block';
    }

    showLoading() {
        this.elements.loading.style.display = 'block';
    }

    hideLoading() {
        this.elements.loading.style.display = 'none';
    }

    showError(message) {
        this.elements.error.textContent = message;
        this.elements.error.style.display = 'block';
    }

    hideError() {
        this.elements.error.style.display = 'none';
    }

    // Utility functions
    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    escapeRegex(text) {
        return text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    }

    // Public API
    getValue() {
        return this.elements.input.value.trim();
    }

    setValue(value) {
        this.elements.input.value = value;
    }

    clear() {
        this.elements.input.value = '';
        this.hideDropdown();
    }

    updateOptions(newOptions) {
        Object.assign(this.options, newOptions);

        // Reload schema if format/type changed
        if (this.canLoadSchema()) {
            this.loadSchema();
        }
    }

    destroy() {
        this.container.innerHTML = '';
    }
}

// Export for use in other modules
if (typeof window !== 'undefined') {
    window.XPathAutocomplete = XPathAutocomplete;
}


if (typeof module !== 'undefined' && module.exports) {
    module.exports = XPathAutocomplete;
}
