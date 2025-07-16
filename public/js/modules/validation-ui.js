// js/modules/validation-ui.js
// UI components for displaying HL7 validation results in ezHealthKonnect

class ValidationUI {
    constructor(containerId, validator) {
        this.container = document.getElementById(containerId);
        this.validator = validator;
        this.currentResults = null;
        this.expandedSections = new Set();
        this.filterSettings = {
            severity: 'all',
            category: 'all',
            segment: 'all'
        };
        this.init();
    }

    /**
     * Initialize the validation UI
     */
    init() {
        if (!this.container) {
            console.error('Validation UI container not found');
            return;
        }
        
        this.container.className = 'validation-ui';
        this.setupStyles();
        this.render();
    }

    /**
     * Setup CSS styles for validation UI
     */
    setupStyles() {
        if (document.getElementById('validation-ui-styles')) return;
        
        const styles = document.createElement('style');
        styles.id = 'validation-ui-styles';
        styles.textContent = `
            .validation-ui {
                font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'Roboto', sans-serif;
                background: #ffffff;
                border-radius: 8px;
                box-shadow: 0 2px 8px rgba(0,0,0,0.1);
                overflow: hidden;
            }

            .validation-header {
                background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
                color: white;
                padding: 20px;
                display: flex;
                justify-content: space-between;
                align-items: center;
            }

            .validation-title {
                font-size: 1.5rem;
                font-weight: 600;
                margin: 0;
            }

            .validation-summary {
                display: flex;
                gap: 20px;
                font-size: 0.9rem;
            }

            .validation-filters {
                background: #f8f9fa;
                padding: 15px 20px;
                border-bottom: 1px solid #dee2e6;
                display: flex;
                gap: 15px;
                align-items: center;
                flex-wrap: wrap;
            }

            .filter-group {
                display: flex;
                align-items: center;
                gap: 8px;
            }

            .filter-group label {
                font-size: 0.875rem;
                font-weight: 500;
                color: #495057;
            }

            .filter-select {
                padding: 6px 12px;
                border: 1px solid #ced4da;
                border-radius: 4px;
                font-size: 0.875rem;
                background: white;
            }

            .validation-content {
                max-height: 600px;
                overflow-y: auto;
            }

            .validation-section {
                border-bottom: 1px solid #dee2e6;
            }

            .section-header {
                background: #f8f9fa;
                padding: 12px 20px;
                display: flex;
                justify-content: space-between;
                align-items: center;
                cursor: pointer;
                transition: background-color 0.2s;
            }

            .section-header:hover {
                background: #e9ecef;
            }

            .section-title {
                font-weight: 600;
                display: flex;
                align-items: center;
                gap: 8px;
            }

            .section-badge {
                display: inline-flex;
                align-items: center;
                padding: 2px 8px;
                border-radius: 12px;
                font-size: 0.75rem;
                font-weight: 500;
                background: #6c757d;
                color: white;
            }

            .expand-icon {
                transition: transform 0.2s;
                color: #6c757d;
            }

            .expand-icon.expanded {
                transform: rotate(90deg);
            }

            .section-content {
                display: none;
                padding: 0;
            }

            .section-content.expanded {
                display: block;
            }

            .validation-item {
                padding: 15px 20px;
                border-bottom: 1px solid #f1f3f4;
                transition: background-color 0.2s;
            }

            .validation-item:hover {
                background: #f8f9fa;
            }

            .validation-item:last-child {
                border-bottom: none;
            }

            .item-header {
                display: flex;
                justify-content: between;
                align-items: flex-start;
                gap: 12px;
                margin-bottom: 8px;
            }

            .severity-icon {
                width: 20px;
                height: 20px;
                border-radius: 50%;
                display: flex;
                align-items: center;
                justify-content: center;
                font-size: 12px;
                font-weight: bold;
                color: white;
                flex-shrink: 0;
                margin-top: 2px;
            }

            .severity-error {
                background: #dc3545;
            }

            .severity-warning {
                background: #fd7e14;
            }

            .severity-info {
                background: #0dcaf0;
            }

            .item-message {
                flex: 1;
                font-weight: 500;
                color: #212529;
                line-height: 1.4;
            }

            .item-details {
                margin-top: 8px;
                font-size: 0.875rem;
                color: #6c757d;
            }

            .item-path {
                background: #e9ecef;
                padding: 4px 8px;
                border-radius: 4px;
                font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
                font-size: 0.8rem;
                display: inline-block;
                margin-right: 8px;
                margin-bottom: 4px;
            }

            .item-value {
                background: #fff3cd;
                padding: 4px 8px;
                border-radius: 4px;
                font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
                font-size: 0.8rem;
                display: inline-block;
                margin-right: 8px;
                margin-bottom: 4px;
                border: 1px solid #ffeaa7;
            }

            .drill-down-btn {
                background: #007bff;
                color: white;
                border: none;
                padding: 4px 8px;
                border-radius: 4px;
                font-size: 0.75rem;
                cursor: pointer;
                transition: background-color 0.2s;
            }

            .drill-down-btn:hover {
                background: #0056b3;
            }

            .suggestions {
                margin-top: 8px;
                padding: 8px 12px;
                background: #d1ecf1;
                border-radius: 4px;
                border-left: 4px solid #bee5eb;
            }

            .suggestions-title {
                font-weight: 500;
                color: #0c5460;
                margin-bottom: 4px;
                font-size: 0.875rem;
            }

            .suggestion-item {
                background: white;
                padding: 4px 8px;
                border-radius: 4px;
                font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
                font-size: 0.8rem;
                display: inline-block;
                margin-right: 8px;
                margin-bottom: 4px;
                border: 1px solid #bee5eb;
                cursor: pointer;
                transition: background-color 0.2s;
            }

            .suggestion-item:hover {
                background: #f1f8f9;
            }

            .no-violations {
                text-align: center;
                padding: 40px 20px;
                color: #28a745;
            }

            .no-violations-icon {
                font-size: 3rem;
                margin-bottom: 10px;
            }

            .empty-state {
                text-align: center;
                padding: 40px 20px;
                color: #6c757d;
            }

            .search-box {
                padding: 8px 12px;
                border: 1px solid #ced4da;
                border-radius: 4px;
                font-size: 0.875rem;
                min-width: 200px;
            }

            .stats-grid {
                display: grid;
                grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
                gap: 15px;
                margin-top: 10px;
            }

            .stat-item {
                text-align: center;
                padding: 8px;
                background: rgba(255,255,255,0.2);
                border-radius: 4px;
            }

            .stat-value {
                font-size: 1.5rem;
                font-weight: bold;
                display: block;
            }

            .stat-label {
                font-size: 0.8rem;
                opacity: 0.9;
            }

            .field-drill-down {
                margin-top: 12px;
                padding: 12px;
                background: #f8f9fa;
                border-radius: 4px;
                border: 1px solid #dee2e6;
            }

            .drill-down-header {
                font-weight: 600;
                margin-bottom: 8px;
                color: #495057;
            }

            .field-components {
                font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
                font-size: 0.875rem;
            }

            .component-item {
                padding: 4px 0;
                border-bottom: 1px solid #dee2e6;
            }

            .component-item:last-child {
                border-bottom: none;
            }

            .component-index {
                color: #6c757d;
                margin-right: 8px;
            }

            .component-value {
                background: #fff;
                padding: 2px 6px;
                border-radius: 3px;
                border: 1px solid #dee2e6;
            }

            .component-empty {
                color: #dc3545;
                font-style: italic;
            }

            .loading-spinner {
                display: inline-block;
                width: 20px;
                height: 20px;
                border: 3px solid #f3f3f3;
                border-top: 3px solid #007bff;
                border-radius: 50%;
                animation: spin 1s linear infinite;
            }

            @keyframes spin {
                0% { transform: rotate(0deg); }
                100% { transform: rotate(360deg); }
            }

            .error-message {
                background: #f8d7da;
                color: #721c24;
                padding: 12px 16px;
                border-radius: 4px;
                margin: 16px;
                border: 1px solid #f5c6cb;
            }

            @media (max-width: 768px) {
                .validation-filters {
                    flex-direction: column;
                    align-items: stretch;
                }

                .filter-group {
                    justify-content: space-between;
                }

                .validation-summary {
                    flex-direction: column;
                    gap: 8px;
                }

                .stats-grid {
                    grid-template-columns: repeat(2, 1fr);
                }
            }
        `;
        
        document.head.appendChild(styles);
    }

    /**
     * Display validation results
     */
    displayResults(validationResults) {
        this.currentResults = validationResults;
        this.render();
    }

    /**
     * Main render method
     */
    render() {
        if (!this.currentResults) {
            this.renderEmptyState();
            return;
        }

        if (!this.currentResults.isValid && this.currentResults.error) {
            this.renderErrorState(this.currentResults.error);
            return;
        }

        this.container.innerHTML = this.buildHTML();
        this.attachEventListeners();
    }

    /**
     * Build main HTML structure
     */
    buildHTML() {
        const results = this.currentResults;
        const filteredResults = this.applyFilters(results);
        
        return `
            ${this.buildHeader(results)}
            ${this.buildFilters()}
            ${this.buildContent(filteredResults)}
        `;
    }

    /**
     * Build header section
     */
    buildHeader(results) {
        const summary = results.summary;
        const isValid = results.isValid;
        
        return `
            <div class="validation-header">
                <div>
                    <h2 class="validation-title">
                        ${isValid ? '✅' : '⚠️'} Validation Results
                        ${results.messageType ? `- ${results.messageType}` : ''}
                    </h2>
                    <div class="stats-grid">
                        <div class="stat-item">
                            <span class="stat-value">${summary.totalFields}</span>
                            <span class="stat-label">Total Fields</span>
                        </div>
                        <div class="stat-item">
                            <span class="stat-value">${summary.validFields}</span>
                            <span class="stat-label">Valid</span>
                        </div>
                        <div class="stat-item">
                            <span class="stat-value">${summary.errors}</span>
                            <span class="stat-label">Errors</span>
                        </div>
                        <div class="stat-item">
                            <span class="stat-value">${summary.warnings}</span>
                            <span class="stat-label">Warnings</span>
                        </div>
                        <div class="stat-item">
                            <span class="stat-value">${summary.missing}</span>
                            <span class="stat-label">Missing</span>
                        </div>
                    </div>
                </div>
                <div class="validation-summary">
                    <div>Status: <strong>${isValid ? 'Valid' : 'Issues Found'}</strong></div>
                </div>
            </div>
        `;
    }

    /**
     * Build filters section
     */
    buildFilters() {
        const severityOptions = [
            { value: 'all', label: 'All Severities' },
            { value: 'error', label: 'Errors Only' },
            { value: 'warning', label: 'Warnings Only' },
            { value: 'info', label: 'Info Only' }
        ];

        const categoryOptions = [
            { value: 'all', label: 'All Categories' },
            { value: 'missing', label: 'Missing Required' },
            { value: 'type', label: 'Type Violations' },
            { value: 'binding', label: 'Binding Deviations' },
            { value: 'healthcare', label: 'Healthcare Rules' }
        ];

        const segmentOptions = [
            { value: 'all', label: 'All Segments' },
            ...this.getAvailableSegments().map(seg => ({ value: seg, label: seg }))
        ];

        return `
            <div class="validation-filters">
                <input type="text" 
                       class="search-box" 
                       placeholder="Search validation issues..." 
                       id="validation-search">
                
                <div class="filter-group">
                    <label for="severity-filter">Severity:</label>
                    <select id="severity-filter" class="filter-select">
                        ${severityOptions.map(opt => 
                            `<option value="${opt.value}" ${this.filterSettings.severity === opt.value ? 'selected' : ''}>
                                ${opt.label}
                            </option>`
                        ).join('')}
                    </select>
                </div>

                <div class="filter-group">
                    <label for="category-filter">Category:</label>
                    <select id="category-filter" class="filter-select">
                        ${categoryOptions.map(opt => 
                            `<option value="${opt.value}" ${this.filterSettings.category === opt.value ? 'selected' : ''}>
                                ${opt.label}
                            </option>`
                        ).join('')}
                    </select>
                </div>

                <div class="filter-group">
                    <label for="segment-filter">Segment:</label>
                    <select id="segment-filter" class="filter-select">
                        ${segmentOptions.map(opt => 
                            `<option value="${opt.value}" ${this.filterSettings.segment === opt.value ? 'selected' : ''}>
                                ${opt.label}
                            </option>`
                        ).join('')}
                    </select>
                </div>
            </div>
        `;
    }

    /**
     * Build content sections
     */
    buildContent(results) {
        if (results.summary.errors === 0 && results.summary.warnings === 0) {
            return this.buildNoViolationsContent();
        }

        const sections = [
            {
                id: 'missing-required',
                title: 'Missing Required Fields',
                items: results.missingRequired,
                icon: '❌',
                severity: 'error'
            },
            {
                id: 'type-violations',
                title: 'Data Type Violations',
                items: results.typeViolations,
                icon: '🔢',
                severity: 'error'
            },
            {
                id: 'binding-deviations',
                title: 'Binding Deviations',
                items: results.bindingDeviations,
                icon: '📋',
                severity: 'warning'
            },
            {
                id: 'healthcare-violations',
                title: 'Healthcare Rule Violations',
                items: results.healthcareViolations,
                icon: '🏥',
                severity: 'warning'
            }
        ];

        const sectionsHTML = sections
            .filter(section => section.items && section.items.length > 0)
            .map(section => this.buildSection(section))
            .join('');

        return `
            <div class="validation-content">
                ${sectionsHTML}
            </div>
        `;
    }

    /**
     * Build individual section
     */
    buildSection(section) {
        const isExpanded = this.expandedSections.has(section.id);
        
        return `
            <div class="validation-section">
                <div class="section-header" data-section="${section.id}">
                    <div class="section-title">
                        <span>${section.icon}</span>
                        <span>${section.title}</span>
                        <span class="section-badge">${section.items.length}</span>
                    </div>
                    <span class="expand-icon ${isExpanded ? 'expanded' : ''}">▶</span>
                </div>
                <div class="section-content ${isExpanded ? 'expanded' : ''}">
                    ${section.items.map(item => this.buildValidationItem(item)).join('')}
                </div>
            </div>
        `;
    }

    /**
     * Build individual validation item
     */
    buildValidationItem(item) {
        const severityClass = `severity-${item.severity || 'info'}`;
        const severityIcon = this.getSeverityIcon(item.severity);
        
        return `
            <div class="validation-item">
                <div class="item-header">
                    <div class="severity-icon ${severityClass}">${severityIcon}</div>
                    <div class="item-message">${this.escapeHtml(item.message)}</div>
                </div>
                ${this.buildItemDetails(item)}
                ${item.suggestions ? this.buildSuggestions(item.suggestions) : ''}
            </div>
        `;
    }

    /**
     * Build item details section
     */
    buildItemDetails(item) {
        let details = '<div class="item-details">';
        
        if (item.path) {
            details += `<span class="item-path">${this.escapeHtml(item.path)}</span>`;
        }
        
        if (item.value !== undefined && item.value !== null) {
            details += `<span class="item-value">${this.escapeHtml(String(item.value))}</span>`;
        }
        
        if (item.code) {
            details += `<span class="item-path">Code: ${this.escapeHtml(item.code)}</span>`;
        }
        
        if (item.path && this.validator) {
            details += `<button class="drill-down-btn" data-path="${this.escapeHtml(item.path)}">🔍 Drill Down</button>`;
        }
        
        details += '</div>';
        
        return details;
    }

    /**
     * Build suggestions section
     */
    buildSuggestions(suggestions) {
        if (!suggestions || suggestions.length === 0) return '';
        
        return `
            <div class="suggestions">
                <div class="suggestions-title">💡 Suggestions:</div>
                ${suggestions.map(suggestion => 
                    `<span class="suggestion-item" data-suggestion="${this.escapeHtml(suggestion.code || suggestion)}">
                        ${this.escapeHtml(suggestion.displayName || suggestion.code || suggestion)}
                    </span>`
                ).join('')}
            </div>
        `;
    }

    /**
     * Build no violations content
     */
    buildNoViolationsContent() {
        return `
            <div class="no-violations">
                <div class="no-violations-icon">✅</div>
                <h3>No Validation Issues Found</h3>
                <p>All fields passed validation successfully!</p>
            </div>
        `;
    }

    /**
     * Render empty state
     */
    renderEmptyState() {
        this.container.innerHTML = `
            <div class="empty-state">
                <h3>No Validation Results</h3>
                <p>Upload and parse an HL7 message to see validation results here.</p>
            </div>
        `;
    }

    /**
     * Render error state
     */
    renderErrorState(error) {
        this.container.innerHTML = `
            <div class="error-message">
                <strong>Validation Error:</strong> ${this.escapeHtml(error)}
            </div>
        `;
    }

    /**
     * Attach event listeners
     */
    attachEventListeners() {
        // Section expand/collapse
        this.container.querySelectorAll('.section-header').forEach(header => {
            header.addEventListener('click', (e) => {
                const sectionId = e.currentTarget.dataset.section;
                this.toggleSection(sectionId);
            });
        });

        // Drill down buttons
        this.container.querySelectorAll('.drill-down-btn').forEach(btn => {
            btn.addEventListener('click', (e) => {
                e.stopPropagation();
                const path = e.target.dataset.path;
                this.showFieldDrillDown(path, e.target);
            });
        });

        // Suggestion clicks
        this.container.querySelectorAll('.suggestion-item').forEach(item => {
            item.addEventListener('click', (e) => {
                const suggestion = e.target.dataset.suggestion;
                this.applySuggestion(suggestion);
            });
        });

        // Filter changes
        ['severity-filter', 'category-filter', 'segment-filter'].forEach(filterId => {
            const element = document.getElementById(filterId);
            if (element) {
                element.addEventListener('change', () => this.updateFilters());
            }
        });

        // Search functionality
        const searchBox = document.getElementById('validation-search');
        if (searchBox) {
            searchBox.addEventListener('input', (e) => {
                this.searchTerm = e.target.value;
                this.applySearch();
            });
        }
    }

    /**
     * Toggle section expansion
     */
    toggleSection(sectionId) {
        if (this.expandedSections.has(sectionId)) {
            this.expandedSections.delete(sectionId);
        } else {
            this.expandedSections.add(sectionId);
        }
        
        const section = this.container.querySelector(`[data-section="${sectionId}"]`);
        const content = section.nextElementSibling;
        const icon = section.querySelector('.expand-icon');
        
        content.classList.toggle('expanded');
        icon.classList.toggle('expanded');
    }

    /**
     * Show field drill-down information
     */
    showFieldDrillDown(fieldPath, buttonElement) {
        if (!this.validator || !this.validator.getFieldDrillDown) {
            console.warn('Drill-down functionality not available');
            return;
        }

        const drillDownInfo = this.validator.getFieldDrillDown(fieldPath);
        if (!drillDownInfo) {
            console.warn(`No drill-down information available for ${fieldPath}`);
            return;
        }

        // Create drill-down display
        const drillDownHTML = this.buildDrillDownDisplay(drillDownInfo);
        
        // Remove existing drill-down displays
        this.container.querySelectorAll('.field-drill-down').forEach(el => el.remove());
        
        // Insert drill-down display
        const drillDownDiv = document.createElement('div');
        drillDownDiv.innerHTML = drillDownHTML;
        buttonElement.closest('.validation-item').appendChild(drillDownDiv.firstElementChild);
    }

    /**
     * Build drill-down display
     */
    buildDrillDownDisplay(drillDownInfo) {
        let content = `
            <div class="field-drill-down">
                <div class="drill-down-header">🔍 Field Analysis: ${drillDownInfo.path}</div>
        `;

        if (drillDownInfo.isComposite && drillDownInfo.components) {
            content += `
                <div class="field-components">
                    <strong>Components:</strong>
                    ${drillDownInfo.components.map(comp => `
                        <div class="component-item">
                            <span class="component-index">${comp.index}:</span>
                            <span class="component-value ${comp.isEmpty ? 'component-empty' : ''}">
                                ${comp.isEmpty ? '(empty)' : this.escapeHtml(comp.value)}
                            </span>
                        </div>
                    `).join('')}
                </div>
            `;
        } else {
            content += `
                <div class="field-components">
                    <strong>Value:</strong> ${this.escapeHtml(String(drillDownInfo.value))}<br>
                    <strong>Type:</strong> ${drillDownInfo.type}
                </div>
            `;
        }

        content += '</div>';
        return content;
    }

    /**
     * Apply suggestion
     */
    applySuggestion(suggestion) {
        // Emit event for parent application to handle
        const event = new CustomEvent('validationSuggestionApplied', {
            detail: { suggestion }
        });
        this.container.dispatchEvent(event);
    }

    /**
     * Update filters
     */
    updateFilters() {
        const severityFilter = document.getElementById('severity-filter');
        const categoryFilter = document.getElementById('category-filter');
        const segmentFilter = document.getElementById('segment-filter');

        if (severityFilter) this.filterSettings.severity = severityFilter.value;
        if (categoryFilter) this.filterSettings.category = categoryFilter.value;
        if (segmentFilter) this.filterSettings.segment = segmentFilter.value;

        this.render();
    }

    /**
     * Apply search
     */
    applySearch() {
        // Re-render with search term applied
        this.render();
    }

    /**
     * Apply filters to results
     */
    applyFilters(results) {
        const filtered = {
            ...results,
            missingRequired: this.filterItems(results.missingRequired, 'missing'),
            typeViolations: this.filterItems(results.typeViolations, 'type'),
            bindingDeviations: this.filterItems(results.bindingDeviations, 'binding'),
            healthcareViolations: this.filterItems(results.healthcareViolations, 'healthcare')
        };

        // Update summary
        filtered.summary = {
            ...results.summary,
            errors: this.countErrors(filtered),
            warnings: this.countWarnings(filtered)
        };

        return filtered;
    }

    /**
     * Filter items based on current settings
     */
    filterItems(items, category) {
        if (!items) return [];

        return items.filter(item => {
            // Severity filter
            if (this.filterSettings.severity !== 'all' && 
                item.severity !== this.filterSettings.severity) {
                return false;
            }

            // Category filter
            if (this.filterSettings.category !== 'all' && 
                this.filterSettings.category !== category) {
                return false;
            }

            // Segment filter
            if (this.filterSettings.segment !== 'all') {
                const itemSegment = this.extractSegmentFromPath(item.path);
                if (itemSegment !== this.filterSettings.segment) {
                    return false;
                }
            }

            // Search filter
            if (this.searchTerm) {
                const searchLower = this.searchTerm.toLowerCase();
                const message = (item.message || '').toLowerCase();
                const path = (item.path || '').toLowerCase();
                const value = String(item.value || '').toLowerCase();
                
                if (!message.includes(searchLower) && 
                    !path.includes(searchLower) && 
                    !value.includes(searchLower)) {
                    return false;
                }
            }

            return true;
        });
    }

    /**
     * Helper methods
     */
    getSeverityIcon(severity) {
        switch (severity) {
            case 'error': return '❌';
            case 'warning': return '⚠️';
            case 'info': return 'ℹ️';
            default: return '•';
        }
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    getAvailableSegments() {
        if (!this.currentResults || !this.currentResults.segments) return [];
        return Object.keys(this.currentResults.segments);
    }

    extractSegmentFromPath(path) {
        if (!path) return '';
        const parts = path.split('.');
        return parts[0] || '';
    }

    countErrors(results) {
        let count = 0;
        count += (results.missingRequired || []).filter(i => i.severity === 'error').length;
        count += (results.typeViolations || []).filter(i => i.severity === 'error').length;
        count += (results.bindingDeviations || []).filter(i => i.severity === 'error').length;
        count += (results.healthcareViolations || []).filter(i => i.severity === 'error').length;
        return count;
    }

    countWarnings(results) {
        let count = 0;
        count += (results.missingRequired || []).filter(i => i.severity === 'warning').length;
        count += (results.typeViolations || []).filter(i => i.severity === 'warning').length;
        count += (results.bindingDeviations || []).filter(i => i.severity === 'warning').length;
        count += (results.healthcareViolations || []).filter(i => i.severity === 'warning').length;
        return count;
    }

    /**
     * Public API methods
     */
    
    // Clear all results
    clear() {
        this.currentResults = null;
        this.expandedSections.clear();
        this.searchTerm = '';
        this.render();
    }

    // Expand all sections
    expandAll() {
        if (!this.currentResults) return;
        
        ['missing-required', 'type-violations', 'binding-deviations', 'healthcare-violations']
            .forEach(sectionId => this.expandedSections.add(sectionId));
        
        this.render();
    }

    // Collapse all sections
    collapseAll() {
        this.expandedSections.clear();
        this.render();
    }

    // Export results as JSON
    exportResults() {
        if (!this.currentResults) return null;
        return JSON.stringify(this.currentResults, null, 2);
    }

    // Get current filter settings
    getFilterSettings() {
        return { ...this.filterSettings };
    }

    // Set filter settings
    setFilterSettings(settings) {
        Object.assign(this.filterSettings, settings);
        this.render();
    }
}

// Export for use in other modules
export { ValidationUI };