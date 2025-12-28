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
            pipelineHasLayers: !!pipeline?.layers,
            pipelineId,
            pipelineVersion
        });

        // Try cache first (if available)
        if (window.variablesCache) {
            const cached = window.variablesCache.get(pipelineId, pipelineVersion);
            if (cached) {
                console.log('⚡ Cache HIT! Using cached variables (instant response)');
                return cached;
            }
        }

        // Convert visual pipeline format (executionGroups) to backend format (flat steps array)
        const backendLayers = {};
        if (pipeline?.layers) {
            for (const [layerKey, layer] of Object.entries(pipeline.layers)) {
                // Flatten execution groups into a simple steps array
                const flatSteps = [];
                if (layer.executionGroups) {
                    layer.executionGroups.forEach(group => {
                        if (group.steps) {
                            flatSteps.push(...group.steps);
                        }
                    });
                }

                backendLayers[layerKey] = {
                    steps: flatSteps
                };
            }
        }

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

        // Store in cache for future requests
        if (window.variablesCache) {
            window.variablesCache.set(pipelineId, pipelineVersion, variables);
        }

        return variables;
    }

    // Calculate pipeline hash for cache versioning (when pipeline.version not available)
    _calculatePipelineHash(pipeline) {
        if (!pipeline?.layers) return 0;

        // Simple hash based on step count and types
        let hash = 0;
        for (const [layerName, layer] of Object.entries(pipeline.layers)) {
            if (layer.executionGroups) {
                layer.executionGroups.forEach(group => {
                    if (group.steps) {
                        group.steps.forEach(step => {
                            // Hash based on step type and config
                            const stepStr = `${step.stepType || ''}:${JSON.stringify(step.config || {})}`;
                            for (let i = 0; i < stepStr.length; i++) {
                                hash = ((hash << 5) - hash) + stepStr.charCodeAt(i);
                                hash |= 0; // Convert to 32-bit integer
                            }
                        });
                    }
                });
            }
        }

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
                    <h4 style="margin: 0; display: flex; align-items: center; gap: 8px;">
                        <span style="font-size: 20px;">📚</span>
                        <span>Available Variables</span>
                    </h4>
                    <button class="close-btn" onclick="window.referencePanel.hide()">✕</button>
                </div>

                <div class="panel-content">
                    ${this.variables.length === 0 ? this.renderEmpty() : this.renderVariables()}
                </div>
            </div>
        `;

        console.log('📝 Setting container HTML, length:', html.length);
        this.container.innerHTML = html;
        console.log('✅ Container HTML set, children count:', this.container.children.length);

        this.attachEventListeners();
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
                    <h4 style="margin: 0;">📚 Available Variables</h4>
                    <button class="close-btn" onclick="window.referencePanel.hide()">✕</button>
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

    // Render variables as simple table
    renderVariables() {
        // Flatten all variables from all categories
        const allVariables = [];
        this.variables.forEach(category => {
            category.variables.forEach(variable => {
                allVariables.push({
                    stepName: category.category,
                    variableName: variable.name,
                    xpath: variable.path,
                    examples: variable.examples || [],
                    description: variable.description || ''
                });
            });
        });

        return `
            <table class="variables-table">
                <thead>
                    <tr>
                        <th>Step Name</th>
                        <th>Variable</th>
                        <th>XPath</th>
                        <th>Usage Examples</th>
                    </tr>
                </thead>
                <tbody>
                    ${allVariables.map(v => `
                        <tr>
                            <td>${v.stepName}</td>
                            <td>${v.variableName}</td>
                            <td>
                                <code class="xpath-code">${v.xpath}</code>
                                <button class="copy-btn-small" data-text="${v.xpath}" title="Copy XPath">📋</button>
                            </td>
                            <td class="examples-cell">
                                ${v.examples.length > 0 ? `
                                    <div class="examples-list">
                                        ${v.examples.map(ex => `
                                            <div class="example-item">
                                                <code class="example-code">${ex}</code>
                                                <button class="copy-btn-tiny" data-text="${ex}" title="Copy">📋</button>
                                            </div>
                                        `).join('')}
                                    </div>
                                ` : '<span style="color: #9ca3af; font-size: 12px;">—</span>'}
                            </td>
                        </tr>
                    `).join('')}
                </tbody>
            </table>
        `;
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
            alert('Failed to copy to clipboard');
        }
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
            border-bottom: 2px solid #e5e7eb;
            display: flex;
            align-items: center;
            justify-content: space-between;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
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
            color: #6366f1;
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
            background: #667eea;
            border: none;
            border-radius: 4px;
            padding: 4px 8px;
            cursor: pointer;
            font-size: 14px;
            transition: all 0.2s;
            flex-shrink: 0;
        }

        .copy-btn-small:hover {
            background: #5568d3;
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
            background: #818cf8;
            border: none;
            border-radius: 3px;
            padding: 2px 6px;
            cursor: pointer;
            font-size: 11px;
            transition: all 0.2s;
            flex-shrink: 0;
        }

        .copy-btn-tiny:hover {
            background: #6366f1;
            transform: scale(1.05);
        }
    `;
    document.head.appendChild(style);
}
