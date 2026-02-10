/**
 * ============================================================================
 * SCRIPT ENRICHMENT EDITOR - Beautiful No-Code Interface
 * ============================================================================
 * Color Scheme: White, Navy Blue, Pastel Pink accents
 * Status Colors: Green (success), Red (error), Yellow (warning)
 *
 * Features:
 * - Side-by-side layout: Variables panel + Code editor
 * - Drag-and-drop variables from panel to editor
 * - Real-time syntax validation
 * - Collapsible variable panel
 * - Beautiful, cohesive design
 * ============================================================================
 */

class ScriptEnrichmentEditor {
    constructor(containerId, options = {}) {
        this.container = document.getElementById(containerId);
        this.options = {
            pipelineId: options.pipelineId || null,
            stepConfig: options.stepConfig || {},
            onSave: options.onSave || null,
            onCancel: options.onCancel || null,
            onChange: options.onChange || null,
            ...options
        };

        this.script = this.options.stepConfig.script || '';
        this.targetPath = this.options.stepConfig.targetPath || 'enriched.script';
        this.timeoutMs = this.options.stepConfig.timeoutMs || 5000;
        this.failOnError = this.options.stepConfig.failOnError || false;

        this.variablePanel = null;
        this.editor = null;
        this.isPanelCollapsed = false;

        this.init();
    }

    init() {
        this.render();
        this.initializeVariablePanel();
        this.attachEventListeners();
    }

    render() {
        if (!this.container) {
            console.error('Script enrichment editor container not found');
            return;
        }

        this.container.innerHTML = `
            <div class="script-enrichment-editor">
                <!-- Main Content: Side-by-side layout -->
                <div class="script-editor-body">
                    <!-- Variable Panel (Left Sidebar) -->
                    <div class="variable-panel-sidebar" id="variablePanelSidebar">
                        <div class="panel-header">
                            <span class="panel-title">
                                <i class="fas fa-cube"></i>
                                Variables
                            </span>
                            <button class="btn-collapse" id="collapsePanel">
                                <i class="fas fa-chevron-left"></i>
                            </button>
                        </div>
                        <div class="panel-body" id="variablePanelContent">
                            <div class="loading-state">
                                <i class="fas fa-spinner fa-spin"></i>
                                <p>Loading variables...</p>
                            </div>
                        </div>
                        <div class="panel-footer">
                            <small>💡 Drag variables into editor</small>
                        </div>
                    </div>

                    <!-- Code Editor (Main Area) -->
                    <div class="code-editor-container">
                        <!-- Editor Toolbar -->
                        <div class="editor-toolbar">
                            <div class="toolbar-left">
                                <button type="button" class="btn-toolbar" id="validateScript">
                                    <i class="fas fa-check-circle"></i>
                                    Validate
                                </button>
                                <button type="button" class="btn-toolbar" id="testScript">
                                    <i class="fas fa-play"></i>
                                    Test Run
                                </button>
                            </div>
                            <div class="toolbar-right">
                                <span class="status-indicator" id="validationStatus">
                                    <i class="fas fa-circle"></i>
                                    Not validated
                                </span>
                            </div>
                        </div>

                        <!-- Code Editor -->
                        <div class="code-editor-wrapper">
                            <textarea
                                id="scriptCodeEditor"
                                class="code-editor"
                                placeholder="// Write your JavaScript code here&#10;// Available: $vars - all variables from previous steps&#10;// Available: input - message data&#10;// Helper functions: getHL7Field(), calculateAge(), etc.&#10;&#10;var riskWeights = $vars['field_mapping.risk_weights'];&#10;&#10;return {&#10;    risk_score: 0,&#10;    risk_level: 'low'&#10;};"
                                spellcheck="false"
                            >${this.escapeHtml(this.script)}</textarea>
                            <div class="line-numbers" id="lineNumbers"></div>
                        </div>

                        <!-- Validation Results -->
                        <div class="validation-results" id="validationResults" style="display: none;"></div>
                    </div>
                </div>

                <!-- Configuration Section -->
                <div class="script-config-section">
                    <div class="config-row">
                        <div class="config-field">
                            <label for="targetPath">
                                <i class="fas fa-bullseye"></i>
                                Target Path
                            </label>
                            <input
                                type="text"
                                id="targetPath"
                                class="form-control"
                                value="${this.targetPath}"
                                placeholder="enriched.script"
                            />
                            <small>Where to store the script result in message data</small>
                        </div>

                        <div class="config-field">
                            <label for="timeoutMs">
                                <i class="fas fa-clock"></i>
                                Timeout (ms)
                            </label>
                            <input
                                type="number"
                                id="timeoutMs"
                                class="form-control"
                                value="${this.timeoutMs}"
                                min="1000"
                                max="30000"
                                step="1000"
                            />
                            <small>Maximum script execution time</small>
                        </div>

                        <div class="config-field config-checkbox">
                            <label class="checkbox-label">
                                <input
                                    type="checkbox"
                                    id="failOnError"
                                    ${this.failOnError ? 'checked' : ''}
                                />
                                <span>Fail pipeline on error</span>
                            </label>
                            <small>Stop pipeline if script fails</small>
                        </div>
                    </div>
                </div>

                <!-- Footer Actions -->
                <div class="script-editor-footer">
                    <div class="footer-left">
                        <button type="button" class="btn-help" id="showHelp">
                            <i class="fas fa-question-circle"></i>
                            Helper Functions
                        </button>
                    </div>
                    <div class="footer-right">
                        <button type="button" class="btn-cancel" id="cancelBtn">
                            Cancel
                        </button>
                        <button type="button" class="btn-save" id="saveBtn">
                            <i class="fas fa-save"></i>
                            Save Configuration
                        </button>
                    </div>
                </div>
            </div>
        `;

        // Update line numbers on load
        this.updateLineNumbers();
    }

    initializeVariablePanel() {
        const panelContainer = document.getElementById('variablePanelContent');
        if (!panelContainer) return;

        // Show helpful message if no pipeline ID
        if (!this.options.pipelineId) {
            panelContainer.innerHTML = `
                <div class="empty-state">
                    <div class="empty-icon">📋</div>
                    <p>Variables will appear here after you add steps to your pipeline</p>
                    <small>Add steps like Field Mapping or Database Enrichment first</small>
                </div>
            `;
            return;
        }

        // Use the working ReferenceVariablesPanel (VariablePanelBuilder needs refactoring)
        try {
            if (!this.options.pipelineBuilder) {
                throw new Error('PipelineBuilder instance required');
            }

            // Initialize ReferenceVariablesPanel with navy blue styling
            if (typeof ReferenceVariablesPanel !== 'undefined') {
                this.variablePanel = new ReferenceVariablesPanel(
                    panelContainer,
                    this.options.pipelineBuilder
                );

                // Show variables for the current step (show all available up to this step)
                // Use the layer and step index passed from PropertiesPanel
                const currentStep = this.options.stepName || 'script_enrichment';
                const layerName = this.options.layerName || 'core'; // Default to 'core' if not provided
                const stepIndex = this.options.stepIndex !== undefined ? this.options.stepIndex : 999;

                console.log('📍 [ScriptEnrichmentEditor] Showing variables for:', {
                    currentStep,
                    layerName,
                    stepIndex,
                    fromOptions: {
                        layerName: this.options.layerName,
                        stepIndex: this.options.stepIndex
                    }
                });

                // Call show() to display variables
                this.variablePanel.show(
                    { stepName: currentStep, stepType: 'pre.enrichment.script' },
                    layerName,
                    stepIndex
                );
            } else {
                throw new Error('ReferenceVariablesPanel not available');
            }
        } catch (error) {
            console.error('Failed to initialize variable panel:', error);
            panelContainer.innerHTML = `
                <div class="error-state">
                    <div class="error-icon">⚠️</div>
                    <p>Failed to load variables</p>
                    <small>${error.message}</small>
                </div>
            `;
        }
    }

    attachEventListeners() {
        const editor = document.getElementById('scriptCodeEditor');
        const togglePanel = document.getElementById('toggleVariablePanel');
        const collapsePanel = document.getElementById('collapsePanel');
        const validateBtn = document.getElementById('validateScript');
        const testBtn = document.getElementById('testScript');
        const saveBtn = document.getElementById('saveBtn');
        const cancelBtn = document.getElementById('cancelBtn');
        const helpBtn = document.getElementById('showHelp');
        const formatBtn = document.getElementById('formatCode');

        // Editor events
        if (editor) {
            editor.addEventListener('input', () => {
                this.script = editor.value;
                this.updateLineNumbers();
                if (this.options.onChange) {
                    this.options.onChange(this.getConfig());
                }
            });

            editor.addEventListener('scroll', () => {
                const lineNumbers = document.getElementById('lineNumbers');
                if (lineNumbers) {
                    lineNumbers.scrollTop = editor.scrollTop;
                }
            });

            // Drop zone for variables
            editor.addEventListener('dragover', (e) => {
                e.preventDefault();
                editor.classList.add('drag-over');
            });

            editor.addEventListener('dragleave', () => {
                editor.classList.remove('drag-over');
            });

            editor.addEventListener('drop', (e) => {
                e.preventDefault();
                editor.classList.remove('drag-over');

                const varData = e.dataTransfer.getData('application/json');
                if (varData) {
                    const variable = JSON.parse(varData);
                    this.insertAtCursor(editor, variable.syntax);
                }
            });
        }

        // Toggle panel
        if (togglePanel) {
            togglePanel.addEventListener('click', () => this.togglePanel());
        }

        if (collapsePanel) {
            collapsePanel.addEventListener('click', () => this.togglePanel());
        }

        // Validate script
        if (validateBtn) {
            validateBtn.addEventListener('click', (e) => {
                e.preventDefault();
                e.stopPropagation();
                this.validateScript();
            });
        }

        // Test script
        if (testBtn) {
            testBtn.addEventListener('click', (e) => {
                e.preventDefault();
                e.stopPropagation();
                this.testScript();
            });
        }

        // Format code
        if (formatBtn) {
            formatBtn.addEventListener('click', (e) => {
                e.preventDefault();
                e.stopPropagation();
                this.formatCode();
            });
        }

        // Save/Cancel
        if (saveBtn) {
            saveBtn.addEventListener('click', () => {
                if (this.options.onSave) {
                    this.options.onSave(this.getConfig());
                }
            });
        }

        if (cancelBtn) {
            cancelBtn.addEventListener('click', () => {
                if (this.options.onCancel) {
                    this.options.onCancel();
                }
            });
        }

        // Help
        if (helpBtn) {
            helpBtn.addEventListener('click', () => this.showHelp());
        }

        // Config fields
        document.getElementById('targetPath')?.addEventListener('input', (e) => {
            this.targetPath = e.target.value;
        });

        document.getElementById('timeoutMs')?.addEventListener('input', (e) => {
            this.timeoutMs = parseInt(e.target.value);
        });

        document.getElementById('failOnError')?.addEventListener('change', (e) => {
            this.failOnError = e.target.checked;
        });
    }

    togglePanel() {
        const sidebar = document.getElementById('variablePanelSidebar');
        const icon = document.querySelector('#collapsePanel i');

        this.isPanelCollapsed = !this.isPanelCollapsed;

        if (this.isPanelCollapsed) {
            sidebar.classList.add('collapsed');
            if (icon) icon.className = 'fas fa-chevron-right';
        } else {
            sidebar.classList.remove('collapsed');
            if (icon) icon.className = 'fas fa-chevron-left';
        }
    }

    updateLineNumbers() {
        const editor = document.getElementById('scriptCodeEditor');
        const lineNumbers = document.getElementById('lineNumbers');

        if (!editor || !lineNumbers) return;

        const lines = editor.value.split('\n').length;
        lineNumbers.innerHTML = Array.from({ length: lines }, (_, i) =>
            `<div class="line-number">${i + 1}</div>`
        ).join('');
    }

    insertAtCursor(element, text) {
        const start = element.selectionStart;
        const end = element.selectionEnd;
        const value = element.value;

        element.value = value.substring(0, start) + text + value.substring(end);
        element.selectionStart = element.selectionEnd = start + text.length;
        element.focus();

        // Trigger input event
        element.dispatchEvent(new Event('input'));
    }

    async validateScript() {
        const statusEl = document.getElementById('validationStatus');
        const resultsEl = document.getElementById('validationResults');

        if (!resultsEl || !statusEl) return;

        // Show validating state
        statusEl.innerHTML = '<i class="fas fa-spinner fa-spin"></i> Validating...';
        statusEl.className = 'status-indicator status-validating';

        try {
            const response = await fetch('/api/fhir/pipeline/validate-script', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ script: this.script })
            });

            const result = await response.json();

            if (result.success) {
                statusEl.innerHTML = '<i class="fas fa-check-circle"></i> Valid';
                statusEl.className = 'status-indicator status-success';
                resultsEl.style.display = 'none';
            } else {
                statusEl.innerHTML = '<i class="fas fa-exclamation-circle"></i> Invalid';
                statusEl.className = 'status-indicator status-error';
                resultsEl.innerHTML = `
                    <div class="validation-error">
                        <i class="fas fa-times-circle"></i>
                        <strong>Syntax Error:</strong> ${result.error || 'Unknown error'}
                    </div>
                `;
                resultsEl.style.display = 'block';
            }
        } catch (error) {
            statusEl.innerHTML = '<i class="fas fa-exclamation-triangle"></i> Error';
            statusEl.className = 'status-indicator status-error';
            resultsEl.innerHTML = `
                <div class="validation-error">
                    <i class="fas fa-times-circle"></i>
                    <strong>Validation failed:</strong> ${error.message}
                </div>
            `;
            resultsEl.style.display = 'block';
        }
    }

    async testScript() {
        // TODO: Implement test execution
        this.showNotification('Test execution feature coming soon!', 'info');
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

    formatCode() {
        const editor = document.getElementById('scriptCodeEditor');
        if (!editor) return;

        try {
            // Basic JavaScript formatting (indent with 2 spaces)
            const lines = editor.value.split('\n');
            let formatted = '';
            let indent = 0;

            lines.forEach(line => {
                const trimmed = line.trim();
                if (trimmed.endsWith('}') || trimmed.endsWith(');')) {
                    indent = Math.max(0, indent - 1);
                }

                formatted += '  '.repeat(indent) + trimmed + '\n';

                if (trimmed.endsWith('{') || trimmed.endsWith('(')) {
                    indent++;
                }
            });

            editor.value = formatted;
            this.script = formatted;
            this.updateLineNumbers();
        } catch (error) {
            console.error('Format error:', error);
        }
    }

    showHelp() {
        const helpers = [
            { title: 'HL7 Field Access', code: 'getHL7Field("PID.5.1")', desc: 'Extract value from HL7 message segment' },
            { title: 'Date Calculations', code: 'calculateAge("19800101")', desc: 'Calculate age from HL7 date format' },
            { title: 'Variable Access (NEW!)', code: '$vars["field_mapping.risk_weights"]', desc: 'Access variables from previous steps' },
            { title: 'Nested Value Access', code: 'getNestedValue("enriched.database.field")', desc: 'Access nested object properties safely' }
        ];

        // Create modal overlay
        const overlay = document.createElement('div');
        overlay.style.cssText = `
            position: fixed; top: 0; left: 0; right: 0; bottom: 0;
            background: rgba(0, 0, 0, 0.5);
            display: flex; align-items: center; justify-content: center;
            z-index: 100000;
        `;

        overlay.innerHTML = `
            <div style="background: white; border-radius: 12px; max-width: 500px; width: 90%; overflow: hidden; box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.1);">
                <div style="padding: 16px 20px; background: #dbeafe; display: flex; align-items: center; justify-content: space-between;">
                    <span style="font-weight: 600; color: #1e293b;">📚 Available Helper Functions</span>
                    <button class="close-btn" style="background: none; border: none; font-size: 20px; cursor: pointer; color: #64748b;">&times;</button>
                </div>
                <div style="padding: 16px 20px; max-height: 400px; overflow-y: auto;">
                    ${helpers.map(h => `
                        <div style="padding: 12px; margin-bottom: 12px; background: #f8fafc; border-radius: 8px; border-left: 3px solid #3b82f6;">
                            <div style="font-weight: 600; color: #1e293b; margin-bottom: 6px;">${h.title}</div>
                            <code style="display: block; font-size: 13px; color: #3b82f6; background: #e0f2fe; padding: 8px 10px; border-radius: 4px; margin-bottom: 6px;">${h.code}</code>
                            <div style="font-size: 13px; color: #64748b;">${h.desc}</div>
                        </div>
                    `).join('')}
                </div>
                <div style="padding: 12px 20px; background: #f8fafc; border-top: 1px solid #e2e8f0; text-align: right;">
                    <button class="got-it-btn" style="padding: 8px 16px; border: none; background: #3b82f6; color: white; border-radius: 6px; cursor: pointer; font-size: 14px;">Got it</button>
                </div>
            </div>
        `;

        document.body.appendChild(overlay);

        const close = () => overlay.remove();
        overlay.querySelector('.close-btn').onclick = close;
        overlay.querySelector('.got-it-btn').onclick = close;
        overlay.onclick = (e) => { if (e.target === overlay) close(); };
    }

    getConfig() {
        return {
            script: this.script,
            targetPath: this.targetPath,
            timeoutMs: this.timeoutMs,
            failOnError: this.failOnError
        };
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }
}

// Export for use in other modules
if (typeof module !== 'undefined' && module.exports) {
    module.exports = ScriptEnrichmentEditor;
}
