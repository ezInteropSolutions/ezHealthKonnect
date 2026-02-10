// ═══════════════════════════════════════════════════════════════
// REFERENCE VARIABLES PANEL - Shows available variables per step
// ═══════════════════════════════════════════════════════════════

class ReferenceVariablesPanel {
    constructor(container, pipelineBuilder) {
        this.container = container;
        this.pipelineBuilder = pipelineBuilder;
        this.currentStep = null;
        this.variables = [];
        this.isVisible = false;
    }

    // Show reference variables for a specific step
    async show(step, layerName, stepIndex) {
        this.currentStep = step;
        this.isVisible = true;

        // Fetch available variables from backend
        try {
            const variables = await this.fetchAvailableVariables(layerName, stepIndex);
            this.variables = variables;
            console.log('📊 Variables received in show():', {
                variablesCount: variables?.length,
                variables: variables,
                isVisible: this.isVisible,
                containerExists: !!this.container
            });
            this.render();
        } catch (error) {
            console.error('Failed to fetch reference variables:', error);
            this.renderError(error.message);
        }
    }

    // Hide the panel
    hide() {
        this.isVisible = false;
        this.container.innerHTML = '';
    }

    // Fetch available variables from backend (with smart caching)
    async fetchAvailableVariables(layerName, stepIndex) {
        const pipeline = this.pipelineBuilder.pipeline;
        const pipelineId = pipeline?.id || 'temp';
        const pipelineVersion = pipeline?.version || this._calculatePipelineHash(pipeline);

        console.log('📡 Fetching reference variables:', {
            layerName,
            stepIndex,
            pipelineHasSteps: !!(pipeline?.getAllSteps || pipeline?.executionGroups),
            pipelineId,
            pipelineVersion
        });

        // TEMPORARILY DISABLED CACHE FOR DEBUGGING - Force fresh fetch every time
        // Try cache first (if available)
        // if (window.variablesCache) {
        //     const cached = window.variablesCache.get(pipelineId, pipelineVersion);
        //     if (cached) {
        //         console.log('⚡ Cache HIT! Using cached variables (instant response)');
        //         return cached;
        //     }
        // }
        console.log('🔄 CACHE DISABLED - Forcing fresh API call');

        // Collect all steps using flat collection pattern
        let allSteps = [];
        if (pipeline?.getAllSteps) {
            allSteps = pipeline.getAllSteps();
        } else if (pipeline?.executionGroups) {
            (pipeline.executionGroups || []).forEach(group => {
                if (group.steps) {
                    allSteps.push(...group.steps);
                }
            });
        }

        // Build backend format with all steps under current layer
        const backendLayers = {};
        backendLayers[layerName] = { steps: allSteps };

        // Build request payload matching backend expectation
        const payload = {
            pipeline: {
                layers: backendLayers
            },
            current_layer: layerName,
            current_step: stepIndex
        };

        console.log('📤 Sending to backend (cache miss):', {
            layerCount: Object.keys(backendLayers).length,
            currentLayer: layerName,
            stepsInCurrentLayer: backendLayers[layerName]?.steps?.length || 0
        });

        const response = await fetch('/api/pipeline/reference-variables', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload)
        });

        if (!response.ok) {
            throw new Error('Failed to fetch reference variables');
        }

        const data = await response.json();
        const variables = data.variables || [];
        console.log('✅ Received variables:', variables.length, 'categories');
        console.log('📋 RAW VARIABLES DATA:', JSON.stringify(variables, null, 2));

        // Store in cache for future requests
        if (window.variablesCache) {
            window.variablesCache.set(pipelineId, pipelineVersion, variables);
        }

        return variables;
    }

    // Calculate pipeline hash for cache versioning (when pipeline.version not available)
    _calculatePipelineHash(pipeline) {
        if (!pipeline) return 0;

        // Collect all steps using flat collection pattern
        let allSteps = [];
        if (pipeline.getAllSteps) {
            allSteps = pipeline.getAllSteps();
        } else if (pipeline.executionGroups) {
            (pipeline.executionGroups || []).forEach(group => {
                if (group.steps) {
                    allSteps.push(...group.steps);
                }
            });
        }

        if (allSteps.length === 0) return 0;

        // Simple hash based on step count and types
        let hash = 0;
        allSteps.forEach(step => {
            // Hash based on step type and config
            const stepStr = `${step.stepType || ''}:${JSON.stringify(step.config || {})}`;
            for (let i = 0; i < stepStr.length; i++) {
                hash = ((hash << 5) - hash) + stepStr.charCodeAt(i);
                hash |= 0; // Convert to 32-bit integer
            }
        });

        return Math.abs(hash);
    }

    // Render the panel
    render() {
        console.log('🎨 Render called:', {
            isVisible: this.isVisible,
            variablesCount: this.variables?.length,
            containerExists: !!this.container,
            container: this.container
        });

        if (!this.isVisible) {
            this.container.innerHTML = '';
            return;
        }

        const html = `
            <div class="reference-variables-panel">
                <div class="panel-header">
                    <h4 style="margin: 0;">Reference Variables</h4>
                </div>

                ${this.variables.length > 0 ? `
                    <div class="search-container">
                        <input type="text"
                               class="search-input"
                               id="variableSearchInput"
                               placeholder="🔍 Search variables..."
                               autocomplete="off">
                    </div>
                ` : ''}

                <div class="panel-content">
                    ${this.variables.length === 0 ? this.renderEmpty() : this.renderVariables()}
                </div>
            </div>
        `;

        console.log('📝 Setting container HTML, length:', html.length);
        this.container.innerHTML = html;
        console.log('✅ Container HTML set, children count:', this.container.children.length);

        this.attachEventListeners();
        this.attachSearchListener();
    }

    // Render empty state
    renderEmpty() {
        return `
            <div class="empty-state">
                <div style="font-size: 48px; opacity: 0.3; margin-bottom: 16px;">📭</div>
                <p style="color: #6b7280; margin: 0;">No variables available yet</p>
                <p style="color: #9ca3af; font-size: 13px; margin: 8px 0 0 0;">
                    Add enrichment steps before this step to access their data
                </p>
            </div>
        `;
    }

    // Render error state
    renderError(message) {
        this.container.innerHTML = `
            <div class="reference-variables-panel">
                <div class="panel-header">
                    <h4 style="margin: 0;">Reference Variables</h4>
                </div>
                <div class="panel-content">
                    <div class="error-state">
                        <div style="font-size: 48px; margin-bottom: 16px;">⚠️</div>
                        <p style="color: #dc2626; margin: 0; font-weight: 500;">Failed to load variables</p>
                        <p style="color: #6b7280; font-size: 13px; margin: 8px 0 0 0;">${message}</p>
                    </div>
                </div>
            </div>
        `;
    }

    // Render variables as compact list (better for narrow sidebars)
    renderVariables() {
        // Group variables by step
        const groupedByStep = {};
        this.variables.forEach(category => {
            const stepName = category.category;
            if (!groupedByStep[stepName]) {
                groupedByStep[stepName] = [];
            }
            category.variables.forEach(variable => {
                groupedByStep[stepName].push({
                    variableName: variable.name,
                    xpath: variable.path,
                    examples: variable.examples || [],
                    description: variable.description || ''
                });
            });
        });

        return `
            <div class="variables-list">
                ${Object.entries(groupedByStep).map(([stepName, variables]) => `
                    <div class="variable-group" data-step="${stepName}">
                        <div class="group-header">${stepName}</div>
                        ${variables.map(v => `
                            <div class="variable-item" data-variable="${v.variableName}" data-xpath="${v.xpath}">
                                <div class="variable-header">
                                    <span class="variable-name">${v.variableName}</span>
                                    <button class="copy-btn-small" data-text="${v.xpath}" title="Copy XPath">📋</button>
                                </div>
                                <code class="xpath-code">${v.xpath}</code>
                                ${v.examples.length > 0 ? `
                                    <div class="examples-container">
                                        ${v.examples.slice(0, 2).map(ex => `
                                            <div class="example-item">
                                                <code class="example-code">${ex}</code>
                                                <button class="copy-btn-tiny" data-text="${ex}" title="Copy">📋</button>
                                            </div>
                                        `).join('')}
                                    </div>
                                ` : ''}
                            </div>
                        `).join('')}
                    </div>
                `).join('')}
            </div>
        `;
    }


    // Attach search listener
    attachSearchListener() {
        const searchInput = this.container.querySelector('#variableSearchInput');
        console.log('🔍 Attaching search listener:', {
            searchInput: searchInput,
            container: this.container
        });

        if (!searchInput) {
            console.warn('⚠️ Search input not found!');
            return;
        }

        searchInput.addEventListener('input', (e) => {
            const searchTerm = e.target.value.toLowerCase().trim();
            console.log('🔎 Search term:', searchTerm);
            this.filterVariables(searchTerm);
        });

        console.log('✅ Search listener attached successfully');
    }

    // Filter variables based on search term
    filterVariables(searchTerm) {
        const variableItems = this.container.querySelectorAll('.variable-item');
        const variableGroups = this.container.querySelectorAll('.variable-group');

        console.log('🔍 Filtering variables:', {
            searchTerm: searchTerm,
            itemCount: variableItems.length,
            groupCount: variableGroups.length
        });

        if (!searchTerm) {
            // Show all
            variableItems.forEach(item => item.style.display = '');
            variableGroups.forEach(group => group.style.display = '');
            console.log('✅ Showing all variables (empty search)');
            return;
        }

        // Hide all first
        variableItems.forEach(item => item.style.display = 'none');
        variableGroups.forEach(group => group.style.display = 'none');

        let matchCount = 0;

        // Show matching items and their groups
        variableItems.forEach(item => {
            // Search in data attributes
            const variableName = (item.getAttribute('data-variable') || '').toLowerCase();
            const xpath = (item.getAttribute('data-xpath') || '').toLowerCase();

            // Also search in the full text content of the item
            const itemText = (item.textContent || '').toLowerCase();

            if (variableName.includes(searchTerm) ||
                xpath.includes(searchTerm) ||
                itemText.includes(searchTerm)) {
                item.style.display = '';
                matchCount++;
                // Show the parent group
                const group = item.closest('.variable-group');
                if (group) group.style.display = '';
            }
        });

        console.log(`✅ Filtered: ${matchCount} matches found`);
    }

    // Attach event listeners
    attachEventListeners() {
        // Copy button click handlers (including tiny copy buttons for examples)
        this.container.querySelectorAll('.copy-btn, .copy-btn-small, .copy-btn-tiny').forEach(btn => {
            btn.addEventListener('click', (e) => {
                e.stopPropagation();
                const text = btn.getAttribute('data-text');
                this.copyToClipboard(text, btn);
            });
        });
    }

    // Copy text to clipboard
    async copyToClipboard(text, button) {
        try {
            await navigator.clipboard.writeText(text);

            // Visual feedback
            const originalText = button.textContent;
            button.textContent = '✅';
            button.style.background = '#10b981';

            setTimeout(() => {
                button.textContent = originalText;
                button.style.background = '';
            }, 1500);
        } catch (error) {
            console.error('Failed to copy:', error);
            this.showNotification('Failed to copy to clipboard', 'error');
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
        setTimeout(() => notification.remove(), 3000);
    }

    // Escape HTML for safe rendering
    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }
}

// CSS Styles (injected once)
if (!document.getElementById('reference-variables-styles')) {
    const style = document.createElement('style');
    style.id = 'reference-variables-styles';
    style.textContent = `
        .reference-variables-panel {
            background: white;
            border-radius: 12px;
            box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1), 0 2px 4px -1px rgba(0, 0, 0, 0.06);
            height: 100%;
            display: flex;
            flex-direction: column;
            overflow: hidden;
        }

        .panel-header {
            padding: 16px 20px;
            border-bottom: 1px solid #e5e7eb;
            display: flex;
            align-items: center;
            justify-content: space-between;
            background: #1e3a8a;
            color: white;
        }

        .panel-header h4 {
            font-size: 16px;
            font-weight: 600;
        }

        .close-btn {
            background: rgba(255, 255, 255, 0.2);
            border: none;
            border-radius: 6px;
            width: 32px;
            height: 32px;
            cursor: pointer;
            color: white;
            font-size: 18px;
            display: flex;
            align-items: center;
            justify-content: center;
            transition: background 0.2s;
        }

        .close-btn:hover {
            background: rgba(255, 255, 255, 0.3);
        }

        .panel-content {
            flex: 1;
            overflow-y: auto;
            padding: 16px;
        }

        .empty-state, .error-state {
            display: flex;
            flex-direction: column;
            align-items: center;
            justify-content: center;
            padding: 48px 24px;
            text-align: center;
        }

        .variables-table {
            width: 100%;
            border-collapse: collapse;
            font-size: 13px;
        }

        .variables-table thead {
            background: #f3f4f6;
            position: sticky;
            top: 0;
            z-index: 1;
        }

        .variables-table th {
            padding: 12px;
            text-align: left;
            font-weight: 600;
            color: #374151;
            border-bottom: 2px solid #e5e7eb;
        }

        .variables-table tbody tr {
            border-bottom: 1px solid #e5e7eb;
            transition: background 0.2s;
        }

        .variables-table tbody tr:hover {
            background: #f9fafb;
        }

        .variables-table td {
            padding: 10px 12px;
            vertical-align: middle;
        }

        .variables-table td:first-child {
            font-weight: 500;
            color: #1e3a8a;
        }

        .variables-table td:nth-child(2) {
            color: #1f2937;
        }

        .variables-table td:nth-child(3) {
            display: flex;
            align-items: center;
            gap: 8px;
        }

        .xpath-code {
            background: #fef3c7;
            padding: 4px 8px;
            border-radius: 4px;
            font-size: 12px;
            color: #92400e;
            font-family: 'Courier New', monospace;
            flex: 1;
        }

        .copy-btn-small {
            background: #fce7f3;
            border: 1px solid #fbcfe8;
            color: #831843;
            border-radius: 4px;
            padding: 4px 8px;
            cursor: pointer;
            font-size: 14px;
            transition: all 0.2s;
            flex-shrink: 0;
        }

        .copy-btn-small:hover {
            background: #fbcfe8;
            border-color: #f9a8d4;
            transform: scale(1.05);
        }

        /* Scrollbar styling */
        .panel-content::-webkit-scrollbar {
            width: 8px;
        }

        .panel-content::-webkit-scrollbar-track {
            background: #f1f1f1;
            border-radius: 4px;
        }

        .panel-content::-webkit-scrollbar-thumb {
            background: #cbd5e1;
            border-radius: 4px;
        }

        .panel-content::-webkit-scrollbar-thumb:hover {
            background: #94a3b8;
        }

        /* Examples column styles */
        .examples-cell {
            padding: 8px 12px !important;
        }

        .examples-list {
            display: flex;
            flex-direction: column;
            gap: 6px;
        }

        .example-item {
            display: flex;
            align-items: center;
            gap: 6px;
        }

        .example-code {
            background: #e0e7ff;
            padding: 3px 6px;
            border-radius: 3px;
            font-size: 11px;
            color: #4338ca;
            font-family: 'Courier New', monospace;
            flex: 1;
            white-space: nowrap;
            overflow: hidden;
            text-overflow: ellipsis;
        }

        .copy-btn-tiny {
            background: #fce7f3;
            border: 1px solid #fbcfe8;
            color: #831843;
            border-radius: 3px;
            padding: 2px 6px;
            cursor: pointer;
            font-size: 11px;
            transition: all 0.2s;
            flex-shrink: 0;
        }

        .copy-btn-tiny:hover {
            background: #fbcfe8;
            border-color: #f9a8d4;
            transform: scale(1.05);
        }

        /* Search Container */
        .search-container {
            padding: 12px 16px;
            border-bottom: 1px solid #e5e7eb;
        }

        .search-input {
            width: 100%;
            padding: 8px 12px;
            border: 1px solid #d1d5db;
            border-radius: 6px;
            font-size: 13px;
            outline: none;
            transition: all 0.2s;
        }

        .search-input:focus {
            border-color: #1e3a8a;
            box-shadow: 0 0 0 3px rgba(30, 58, 138, 0.1);
        }

        /* Variables List (Compact Layout) */
        .variables-list {
            display: flex;
            flex-direction: column;
            gap: 16px;
        }

        .variable-group {
            border-bottom: 1px solid #e5e7eb;
            padding-bottom: 12px;
        }

        .variable-group:last-child {
            border-bottom: none;
        }

        .group-header {
            font-size: 12px;
            font-weight: 600;
            color: #1e3a8a;
            text-transform: uppercase;
            letter-spacing: 0.5px;
            margin-bottom: 8px;
            padding-bottom: 4px;
            border-bottom: 2px solid #1e3a8a;
        }

        .variable-item {
            padding: 8px;
            margin-bottom: 8px;
            background: #f9fafb;
            border-radius: 6px;
            border-left: 3px solid #fce7f3;
            transition: all 0.2s;
        }

        .variable-item:hover {
            background: #f3f4f6;
            border-left-color: #f9a8d4;
        }

        .variable-header {
            display: flex;
            align-items: center;
            justify-content: space-between;
            margin-bottom: 4px;
        }

        .variable-name {
            font-weight: 600;
            font-size: 13px;
            color: #1f2937;
        }

        .variable-item .xpath-code {
            display: block;
            background: #fef3c7;
            padding: 4px 8px;
            border-radius: 4px;
            font-size: 11px;
            color: #92400e;
            font-family: 'Courier New', monospace;
            margin: 4px 0;
            word-break: break-all;
        }

        .examples-container {
            margin-top: 6px;
            padding-top: 6px;
            border-top: 1px solid #e5e7eb;
        }

        .example-item {
            display: flex;
            align-items: center;
            gap: 6px;
            margin-bottom: 4px;
        }

        .example-code {
            flex: 1;
            background: #eff6ff;
            padding: 3px 6px;
            border-radius: 3px;
            font-size: 11px;
            color: #1e40af;
            font-family: 'Courier New', monospace;
            word-break: break-all;
        }

        .copy-btn-small {
            background: #fce7f3;
            border: 1px solid #fbcfe8;
            color: #831843;
            border-radius: 4px;
            padding: 4px 8px;
            cursor: pointer;
            font-size: 14px;
            transition: all 0.2s;
            flex-shrink: 0;
        }

        .copy-btn-small:hover {
            background: #fbcfe8;
            border-color: #f9a8d4;
            transform: scale(1.05);
        }
    `;
    document.head.appendChild(style);
}
