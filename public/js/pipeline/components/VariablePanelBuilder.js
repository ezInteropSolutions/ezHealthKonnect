/**
 * ============================================================================
 * VARIABLE PANEL BUILDER - NO-CODE DRAG & DROP
 * ============================================================================
 * Provides IntelliSense-style variable browser with drag-and-drop support
 * Users can drag variables directly into script editors or formula builders
 *
 * Design Philosophy:
 * - Zero JavaScript knowledge required
 * - Visual, intuitive interface
 * - Type-aware (shows icons for string, number, object, array)
 * - Grouped by step for easy navigation
 * ============================================================================
 */

class VariablePanelBuilder {
    constructor(containerId, options = {}) {
        this.container = document.getElementById(containerId);
        this.options = {
            pipelineId: options.pipelineId || null,
            onVariableSelect: options.onVariableSelect || null,
            onVariableDrag: options.onVariableDrag || null,
            showSearch: options.showSearch !== false, // Default true
            showTypes: options.showTypes !== false,   // Default true
            collapsible: options.collapsible !== false, // Default true
            ...options
        };

        this.variables = [];
        this.groupedVariables = {};
        this.selectedVariable = null;
        this.searchTerm = '';

        this.init();
    }

    init() {
        this.render();
        if (this.options.pipelineId) {
            this.loadVariables(this.options.pipelineId);
        }
    }

    /**
     * Load variables from API for given pipeline
     */
    async loadVariables(pipelineId, sampleMessage = null) {
        try {
            const response = await fetch('/api/pipelines/variables', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    pipeline_id: pipelineId,
                    message: sampleMessage
                })
            });

            if (!response.ok) {
                throw new Error(`Failed to load variables: ${response.statusText}`);
            }

            const data = await response.json();
            this.variables = data.variables || [];
            this.groupedVariables = data.grouped_by_step || {};

            this.renderVariableList();
        } catch (error) {
            console.error('Error loading variables:', error);
            this.showError('Failed to load variables. Please try again.');
        }
    }

    /**
     * Render the container structure
     */
    render() {
        if (!this.container) {
            console.error('Variable panel container not found');
            return;
        }

        this.container.innerHTML = `
            <div class="variable-panel">
                <div class="variable-panel-header">
                    <h4>
                        <i class="fas fa-cube"></i>
                        Available Variables
                    </h4>
                    ${this.options.showSearch ? `
                        <div class="variable-search">
                            <i class="fas fa-search"></i>
                            <input
                                type="text"
                                id="variableSearch"
                                placeholder="Search variables..."
                                class="form-control form-control-sm"
                            />
                        </div>
                    ` : ''}
                </div>
                <div class="variable-panel-body" id="variableList">
                    <div class="text-center text-muted p-3">
                        <i class="fas fa-spinner fa-spin"></i>
                        Loading variables...
                    </div>
                </div>
            </div>
        `;

        // Attach event listeners
        if (this.options.showSearch) {
            const searchInput = document.getElementById('variableSearch');
            if (searchInput) {
                searchInput.addEventListener('input', (e) => {
                    this.searchTerm = e.target.value.toLowerCase();
                    this.renderVariableList();
                });
            }
        }
    }

    /**
     * Render the variable list grouped by step
     */
    renderVariableList() {
        const listContainer = document.getElementById('variableList');
        if (!listContainer) return;

        // Filter variables based on search
        const filteredGroups = this.filterVariables();

        if (Object.keys(filteredGroups).length === 0) {
            listContainer.innerHTML = `
                <div class="text-center text-muted p-3">
                    <i class="fas fa-inbox"></i>
                    <p class="mt-2">No variables found</p>
                </div>
            `;
            return;
        }

        // Build HTML for grouped variables
        let html = '<div class="variable-groups">';

        Object.keys(filteredGroups).sort().forEach(stepName => {
            const vars = filteredGroups[stepName];
            const stepDisplayName = this.formatStepName(stepName);
            const stepId = stepName.replace(/[^a-z0-9]/gi, '_');

            html += `
                <div class="variable-group" data-step="${stepName}">
                    <div class="variable-group-header ${this.options.collapsible ? 'collapsible' : ''}"
                         data-toggle="collapse"
                         data-target="#vars_${stepId}">
                        <i class="fas fa-chevron-down collapse-icon"></i>
                        <span class="step-icon">${this.getStepIcon(vars[0]?.step_type)}</span>
                        <strong>${stepDisplayName}</strong>
                        <span class="badge badge-secondary ml-2">${vars.length}</span>
                    </div>
                    <div id="vars_${stepId}" class="collapse show variable-group-body">
                        ${vars.map(v => this.renderVariableItem(v)).join('')}
                    </div>
                </div>
            `;
        });

        html += '</div>';
        listContainer.innerHTML = html;

        // Attach drag event listeners
        this.attachDragListeners();
    }

    /**
     * Render individual variable item with drag support
     */
    renderVariableItem(variable) {
        const typeIcon = this.getTypeIcon(variable.type);
        const typeColor = this.getTypeColor(variable.type);
        const varPath = variable.full_path;
        const varName = variable.name;
        const sampleValue = this.formatSampleValue(variable.sample_value, variable.type);

        return `
            <div class="variable-item"
                 draggable="true"
                 data-var-path="${varPath}"
                 data-var-name="${varName}"
                 data-var-type="${variable.type}"
                 title="Drag to use: $vars['${varPath}']">
                <div class="variable-item-main">
                    <span class="type-icon" style="color: ${typeColor}">${typeIcon}</span>
                    <span class="variable-name">${varName}</span>
                    ${this.options.showTypes ? `
                        <span class="badge badge-sm badge-${this.getTypeBadgeClass(variable.type)}">${variable.type}</span>
                    ` : ''}
                    <button class="btn-copy-var" data-var-path="${varPath}" title="Copy to clipboard">
                        <i class="fas fa-copy"></i>
                    </button>
                </div>
                ${sampleValue ? `
                    <div class="variable-sample">
                        <code>${sampleValue}</code>
                    </div>
                ` : ''}
            </div>
        `;
    }

    /**
     * Filter variables based on search term
     */
    filterVariables() {
        if (!this.searchTerm) {
            return this.groupedVariables;
        }

        const filtered = {};
        Object.keys(this.groupedVariables).forEach(stepName => {
            const matchingVars = this.groupedVariables[stepName].filter(v =>
                v.name.toLowerCase().includes(this.searchTerm) ||
                v.full_path.toLowerCase().includes(this.searchTerm) ||
                stepName.toLowerCase().includes(this.searchTerm)
            );

            if (matchingVars.length > 0) {
                filtered[stepName] = matchingVars;
            }
        });

        return filtered;
    }

    /**
     * Attach drag-and-drop event listeners
     */
    attachDragListeners() {
        const items = document.querySelectorAll('.variable-item');
        items.forEach(item => {
            // Drag start
            item.addEventListener('dragstart', (e) => {
                const varPath = item.getAttribute('data-var-path');
                const varName = item.getAttribute('data-var-name');
                const varType = item.getAttribute('data-var-type');

                // Set drag data (multiple formats for compatibility)
                e.dataTransfer.effectAllowed = 'copy';
                e.dataTransfer.setData('text/plain', `$vars["${varPath}"]`);
                e.dataTransfer.setData('application/json', JSON.stringify({
                    path: varPath,
                    name: varName,
                    type: varType,
                    syntax: `$vars["${varPath}"]`
                }));

                // Visual feedback
                item.classList.add('dragging');

                // Callback
                if (this.options.onVariableDrag) {
                    this.options.onVariableDrag({ path: varPath, name: varName, type: varType });
                }
            });

            // Drag end
            item.addEventListener('dragend', (e) => {
                item.classList.remove('dragging');
            });

            // Click to select
            item.addEventListener('click', (e) => {
                if (!e.target.closest('.btn-copy-var')) {
                    this.selectVariable(item.getAttribute('data-var-path'));
                }
            });
        });

        // Copy button handlers
        document.querySelectorAll('.btn-copy-var').forEach(btn => {
            btn.addEventListener('click', (e) => {
                e.stopPropagation();
                const varPath = btn.getAttribute('data-var-path');
                this.copyToClipboard(`$vars["${varPath}"]`);
            });
        });
    }

    /**
     * Select a variable (highlight and callback)
     */
    selectVariable(varPath) {
        // Remove previous selection
        document.querySelectorAll('.variable-item').forEach(item => {
            item.classList.remove('selected');
        });

        // Add selection to clicked item
        const item = document.querySelector(`.variable-item[data-var-path="${varPath}"]`);
        if (item) {
            item.classList.add('selected');
            this.selectedVariable = varPath;

            // Callback
            if (this.options.onVariableSelect) {
                const varName = item.getAttribute('data-var-name');
                const varType = item.getAttribute('data-var-type');
                this.options.onVariableSelect({ path: varPath, name: varName, type: varType });
            }
        }
    }

    /**
     * Copy variable syntax to clipboard
     */
    copyToClipboard(text) {
        navigator.clipboard.writeText(text).then(() => {
            // Show toast notification
            this.showToast('Copied to clipboard!', 'success');
        }).catch(err => {
            console.error('Failed to copy:', err);
            this.showToast('Failed to copy', 'error');
        });
    }

    /**
     * Utility: Format step name for display
     */
    formatStepName(stepName) {
        return stepName
            .split('_')
            .map(word => word.charAt(0).toUpperCase() + word.slice(1))
            .join(' ');
    }

    /**
     * Utility: Get icon for step type
     */
    getStepIcon(stepType) {
        const icons = {
            'pre.enrichment.database': '🗄️',
            'pre.enrichment.api': '🌐',
            'pre.enrichment.script': '📜',
            'core.transformation': '🔄',
            'core.mapping': '🗺️',
            'validation': '✅',
            'control.conditional': '🔀'
        };
        return icons[stepType] || '⚙️';
    }

    /**
     * Utility: Get icon for variable type
     */
    getTypeIcon(type) {
        const icons = {
            'string': '📝',
            'number': '🔢',
            'boolean': '✓',
            'object': '{}',
            'array': '[]',
            'date': '📅',
            'unknown': '❓'
        };
        return icons[type] || icons['unknown'];
    }

    /**
     * Utility: Get color for variable type
     */
    getTypeColor(type) {
        const colors = {
            'string': '#28a745',
            'number': '#007bff',
            'boolean': '#6f42c1',
            'object': '#fd7e14',
            'array': '#ffc107',
            'date': '#20c997',
            'unknown': '#6c757d'
        };
        return colors[type] || colors['unknown'];
    }

    /**
     * Utility: Get badge class for variable type
     */
    getTypeBadgeClass(type) {
        const classes = {
            'string': 'success',
            'number': 'primary',
            'boolean': 'purple',
            'object': 'warning',
            'array': 'info',
            'date': 'teal',
            'unknown': 'secondary'
        };
        return classes[type] || classes['unknown'];
    }

    /**
     * Utility: Format sample value for display
     */
    formatSampleValue(value, type) {
        if (value === null || value === undefined) return null;

        if (type === 'object') return '{...}';
        if (type === 'array') return '[...]';
        if (typeof value === 'string' && value.length > 50) {
            return value.substring(0, 50) + '...';
        }

        return JSON.stringify(value);
    }

    /**
     * Show error message
     */
    showError(message) {
        const listContainer = document.getElementById('variableList');
        if (listContainer) {
            listContainer.innerHTML = `
                <div class="alert alert-danger m-3">
                    <i class="fas fa-exclamation-triangle"></i>
                    ${message}
                </div>
            `;
        }
    }

    /**
     * Show toast notification
     */
    showToast(message, type = 'info') {
        // Create toast element
        const toast = document.createElement('div');
        toast.className = `toast-notification toast-${type}`;
        toast.innerHTML = `
            <i class="fas fa-${type === 'success' ? 'check' : 'info'}-circle"></i>
            ${message}
        `;

        document.body.appendChild(toast);

        // Animate in
        setTimeout(() => toast.classList.add('show'), 10);

        // Remove after 2 seconds
        setTimeout(() => {
            toast.classList.remove('show');
            setTimeout(() => toast.remove(), 300);
        }, 2000);
    }

    /**
     * Public API: Refresh variables
     */
    refresh(pipelineId = null, sampleMessage = null) {
        const id = pipelineId || this.options.pipelineId;
        if (id) {
            this.loadVariables(id, sampleMessage);
        }
    }

    /**
     * Public API: Get selected variable
     */
    getSelectedVariable() {
        return this.selectedVariable;
    }

    /**
     * Public API: Clear selection
     */
    clearSelection() {
        document.querySelectorAll('.variable-item').forEach(item => {
            item.classList.remove('selected');
        });
        this.selectedVariable = null;
    }
}

// Export for use in other modules
if (typeof module !== 'undefined' && module.exports) {
    module.exports = VariablePanelBuilder;
}
