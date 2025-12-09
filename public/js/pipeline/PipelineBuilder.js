/**
 * Pipeline Builder - Main Orchestrator
 * Coordinates all managers and handles pipeline operations
 */

class PipelineBuilder {
    constructor() {
        this.pipeline = null;
        this.interfaceId = null;
        this.messageType = null;
        this.isSaved = true;

        // Initialize managers
        this.dragDropManager = null;
        this.canvasRenderer = null;
        this.stepNodeManager = null;
        this.toolboxManager = null;
        this.propertiesPanel = null;
        this.layerContainer = null;

        this.init();
    }

    async init() {
        // Parse URL parameters
        this.parseURLParams();

        // Initialize managers
        this.initializeManagers();

        // Load or create pipeline
        await this.loadPipeline();

        // Setup UI event listeners
        this.setupEventListeners();

        // Setup auto-save
        this.setupAutoSave();

        console.log('Pipeline Builder initialized');
    }

    /**
     * Parse URL parameters
     */
    parseURLParams() {
        const params = new URLSearchParams(window.location.search);
        this.interfaceId = params.get('interfaceId');
        this.messageType = params.get('messageType');
        const pipelineId = params.get('pipelineId');

        if (pipelineId) {
            this.pipelineId = pipelineId;
        }

        // Update header info
        this.updateHeaderInfo();
    }

    /**
     * Initialize all managers
     */
    initializeManagers() {
        this.dragDropManager = new DragDropManager(this);
        this.canvasRenderer = new CanvasRenderer(this);
        this.stepNodeManager = new StepNodeManager(this);
        this.propertiesPanel = new PropertiesPanel(this);
        this.layerContainer = new LayerContainer(this);
        this.toolboxManager = new ToolboxManager(this);

        // Make PropertiesPanel globally accessible for row click handlers
        window.propertiesPanel = this.propertiesPanel;
    }

    /**
     * Load or create pipeline
     */
    async loadPipeline() {
        try {
            if (this.pipelineId) {
                // Load existing pipeline by ID
                this.pipeline = await window.pipelineAPI.loadPipeline(this.pipelineId);
                this.interfaceId = this.pipeline.interfaceId;
                this.messageType = this.pipeline.messageType;
            } else if (this.interfaceId) {
                // Load interface to get message type if not provided
                if (!this.messageType || this.messageType === 'hl7v2') {
                    console.log('📡 Loading interface to get message type...');
                    const interfaceResponse = await fetch(`/api/interfaces/${this.interfaceId}`);
                    if (interfaceResponse.ok) {
                        const interfaceData = await interfaceResponse.json();
                        // Use message_type from interface, default to ADT^A01 if not set or invalid
                        const dbMessageType = interfaceData.message_type || interfaceData.messageType;
                        this.messageType = (dbMessageType && dbMessageType !== 'hl7v2')
                            ? dbMessageType
                            : 'ADT^A01';
                        console.log(`✅ Message type resolved: ${this.messageType}`);
                    } else {
                        console.warn('⚠️ Failed to load interface, defaulting to ADT^A01');
                        this.messageType = 'ADT^A01';
                    }
                }

                // Try to load existing pipeline for interface/message type
                this.pipeline = await window.pipelineAPI.loadPipelineByInterface(
                    this.interfaceId,
                    this.messageType
                );

                if (!this.pipeline) {
                    // Create new pipeline
                    this.pipeline = new VisualPipeline({
                        interfaceId: this.interfaceId,
                        messageType: this.messageType,
                        name: `${this.messageType} Pipeline`
                    });
                }
            } else {
                // Create blank pipeline
                this.pipeline = new VisualPipeline({
                    name: 'New Pipeline'
                });
            }

            // Render pipeline
            this.renderPipeline();
            this.isSaved = true;

        } catch (error) {
            console.error('Failed to load pipeline:', error);
            this.pipeline = new VisualPipeline({
                name: 'New Pipeline'
            });
            this.renderPipeline();
        }
    }

    /**
     * Render pipeline to canvas
     */
    renderPipeline() {
        this.layerContainer.renderAllLayers();
        this.updateHeaderInfo();

        // Redraw connections after render
        setTimeout(() => {
            this.canvasRenderer.redrawAllConnections();
        }, 100);
    }

    /**
     * Update header info
     */
    updateHeaderInfo() {
        const titleEl = document.getElementById('pipelineTitle');
        const infoEl = document.getElementById('interfaceInfo');

        if (titleEl && this.pipeline) {
            titleEl.textContent = this.pipeline.name;
        }

        if (infoEl && this.messageType) {
            infoEl.textContent = `${this.messageType}`;
            infoEl.style.display = 'inline-block';
        }
    }

    /**
     * Setup event listeners
     */
    setupEventListeners() {
        // Back button
        const backBtn = document.getElementById('backBtn');
        if (backBtn) {
            backBtn.addEventListener('click', () => this.navigateBack());
        }

        // Save button
        const saveBtn = document.getElementById('savePipelineBtn');
        if (saveBtn) {
            saveBtn.addEventListener('click', () => this.savePipeline());
        }

        // Test button
        const testBtn = document.getElementById('testPipelineBtn');
        if (testBtn) {
            testBtn.addEventListener('click', () => this.openTestModal());
        }

        // Execution mode toggle
        const parallelBtn = document.getElementById('parallelModeBtn');
        const inlineBtn = document.getElementById('inlineModeBtn');

        if (parallelBtn) {
            parallelBtn.addEventListener('click', () => this.setExecutionMode('parallel'));
        }

        if (inlineBtn) {
            inlineBtn.addEventListener('click', () => this.setExecutionMode('inline'));
        }

        // Auto layout
        const autoLayoutBtn = document.getElementById('autoLayoutBtn');
        if (autoLayoutBtn) {
            autoLayoutBtn.addEventListener('click', () => this.canvasRenderer.autoLayout());
        }

        // Clear canvas
        const clearBtn = document.getElementById('clearCanvasBtn');
        if (clearBtn) {
            clearBtn.addEventListener('click', () => this.clearCanvas());
        }

        // Test modal
        this.setupTestModal();

        // Prevent accidental navigation
        window.addEventListener('beforeunload', (e) => {
            if (!this.isSaved) {
                e.preventDefault();
                e.returnValue = 'You have unsaved changes. Are you sure you want to leave?';
                return e.returnValue;
            }
        });
    }

    /**
     * Setup test modal
     */
    setupTestModal() {
        const modal = document.getElementById('testModal');
        const closeButtons = modal?.querySelectorAll('.modal-close');
        const runTestBtn = document.getElementById('runTestBtn');

        closeButtons?.forEach(btn => {
            btn.addEventListener('click', () => {
                modal.classList.remove('active');
            });
        });

        if (runTestBtn) {
            runTestBtn.addEventListener('click', () => this.runTest());
        }

        // Close on outside click
        modal?.addEventListener('click', (e) => {
            if (e.target === modal) {
                modal.classList.remove('active');
            }
        });
    }

    /**
     * Open test modal
     */
    openTestModal() {
        const modal = document.getElementById('testModal');
        if (modal) {
            // Reset test results when opening modal
            const resultsDiv = document.getElementById('testResults');
            const resultsContent = document.getElementById('testResultsContent');
            if (resultsDiv) resultsDiv.style.display = 'none';
            if (resultsContent) resultsContent.innerHTML = '';

            modal.classList.add('active');
        }
    }

    /**
     * Run pipeline test
     */
    async runTest() {
        const messageInput = document.getElementById('testMessageInput');
        const resultsDiv = document.getElementById('testResults');
        const resultsContent = document.getElementById('testResultsContent');

        if (!messageInput || !resultsDiv || !resultsContent) return;

        const sampleMessage = messageInput.value.trim();

        if (!sampleMessage) {
            this.dragDropManager.showNotification('Please enter a sample message', 'warning');
            return;
        }

        try {
            resultsContent.innerHTML = '<p style="text-align: center;"><i class="fas fa-spinner fa-spin"></i> Running test...</p>';
            resultsDiv.style.display = 'block';

            const result = await window.pipelineAPI.testPipeline(this.pipeline, sampleMessage);

            // Display results with enhanced FHIR resource rendering
            resultsContent.innerHTML = this.renderTestResults(result);

            this.dragDropManager.showNotification(
                result.success ? 'Test passed' : 'Test failed',
                result.success ? 'success' : 'error'
            );

        } catch (error) {
            console.error('Test execution error:', error);

            // Extract meaningful error message
            let errorMessage = 'Unknown error occurred';
            if (error.message) {
                errorMessage = error.message;
            } else if (typeof error === 'string') {
                errorMessage = error;
            } else if (error.error) {
                errorMessage = error.error;
            } else {
                errorMessage = JSON.stringify(error, null, 2);
            }

            resultsContent.innerHTML = `
                <div class="test-result error">
                    <h4><i class="fas fa-times-circle"></i> Test Error</h4>
                    <p>${this.escapeHtml(errorMessage)}</p>
                    <details style="margin-top: 0.5rem;">
                        <summary style="cursor: pointer; font-size: 0.875rem;">View Error Details</summary>
                        <pre style="background: #fef2f2; padding: 0.5rem; border-radius: 0.25rem; overflow: auto; max-height: 300px; font-size: 0.75rem;">${this.escapeHtml(JSON.stringify(error, null, 2))}</pre>
                    </details>
                </div>
            `;
            this.dragDropManager.showNotification('Test failed: ' + errorMessage, 'error');
        }
    }

    /**
     * Render test results with proper FHIR resource display
     */
    renderTestResults(result) {
        // Extract data from correct structure (Go backend returns execution_results, final_output, errors)
        const stepsExecuted = result.execution_results?.length || 0;
        const finalOutput = result.final_output || {};
        const validationErrors = result.errors || [];

        // Find the mapping step to get resources and validation errors
        let fhirBundle = null;
        let resourcesCreated = [];
        let mappingValidationErrors = [];

        if (result.execution_results) {
            const mappingStep = result.execution_results.find(r => r.step_type === 'core.mapping');
            if (mappingStep && mappingStep.output) {
                fhirBundle = mappingStep.output.fhir_bundle;
                resourcesCreated = mappingStep.output.resources_created || [];
                mappingValidationErrors = mappingStep.output.validation_errors || [];
            }
        }

        // Also check final_output for bundle
        if (!fhirBundle && finalOutput.fhir_bundle) {
            fhirBundle = finalOutput.fhir_bundle;
        }

        let html = `
            <div style="margin-bottom: 15px; text-align: right;">
                <button onclick="window.pipelineBuilder.runTest()" class="btn-secondary" style="padding: 8px 16px;">
                    <i class="fas fa-redo"></i> Run Test Again
                </button>
            </div>
            <div class="test-result ${result.success ? 'success' : 'error'}">
                <h4>
                    <i class="fas fa-${result.success ? 'check-circle' : 'times-circle'}"></i>
                    ${result.success ? 'Test Passed' : 'Test Failed'}
                </h4>
                <p><strong>Execution Time:</strong> ${result.execution_time_ms || 'N/A'}ms</p>
                <p><strong>Steps Executed:</strong> ${stepsExecuted}</p>
                ${result.error ? `<p class="error-message"><strong>Error:</strong> ${this.escapeHtml(result.error)}</p>` : ''}
            </div>
        `;

        // Show validation errors prominently if any
        const allErrors = [...validationErrors, ...mappingValidationErrors];
        if (allErrors.length > 0) {
            html += this.renderValidationErrors(allErrors);
        }

        // Render transformed message output (FHIR Bundle)
        if (fhirBundle) {
            html += this.renderTransformedMessage(fhirBundle);
        }

        // Render FHIR resources with narratives
        if (resourcesCreated && Array.isArray(resourcesCreated) && resourcesCreated.length > 0) {
            html += this.renderFHIRResources(resourcesCreated);
        }

        // Copy buttons
        html += `
            <div style="margin-top: 1rem; display: flex; gap: 0.5rem;">
                <button id="copyBundleBtn" class="btn-copy">
                    <i class="fas fa-copy"></i> Copy FHIR Bundle
                </button>
                <button id="copyResultsBtn" class="btn btn-secondary">
                    <i class="fas fa-file-code"></i> Copy Full Results
                </button>
            </div>
        `;

        // Application details in collapsible section
        html += `
            <details style="margin-top: 1rem;">
                <summary style="cursor: pointer; font-weight: 600; color: #64748b;">
                    <i class="fas fa-cog"></i> Application Details
                </summary>
                <pre id="fullResultsJSON" style="background: #f1f5f9; padding: 1rem; border-radius: 0.375rem; overflow: auto; max-height: 400px; font-size: 0.75rem;">${this.escapeHtml(JSON.stringify(result, null, 2))}</pre>
            </details>
        `;

        // Setup copy buttons after rendering
        setTimeout(() => {
            const copyBundleBtn = document.getElementById('copyBundleBtn');
            if (copyBundleBtn) {
                copyBundleBtn.addEventListener('click', () => this.copyBundleToClipboard(fhirBundle));
            }

            const copyBtn = document.getElementById('copyResultsBtn');
            if (copyBtn) {
                copyBtn.addEventListener('click', () => this.copyResultsToClipboard(result));
            }
        }, 100);

        return html;
    }

    /**
     * Render transformed FHIR message output
     */
    renderTransformedMessage(bundle) {
        if (!bundle) return '';

        return `
            <div class="transformed-message-section">
                <h5>
                    <i class="fas fa-exchange-alt"></i>
                    Transformed FHIR Bundle
                </h5>
                <div style="margin-bottom: 1rem; color: #64748b; font-size: 0.875rem;">
                    <strong>Type:</strong> ${bundle.type || 'N/A'} |
                    <strong>Resources:</strong> ${bundle.entry?.length || 0} |
                    <strong>Timestamp:</strong> ${bundle.timestamp || 'N/A'}
                </div>
                <div class="message-output">
                    <pre style="margin: 0; white-space: pre-wrap; word-wrap: break-word;">${this.escapeHtml(JSON.stringify(bundle, null, 2))}</pre>
                </div>
            </div>
        `;
    }

    /**
     * Render FHIR resources with proper HTML narrative display
     */
    renderFHIRResources(resources) {
        let html = `
            <div class="fhir-resources-section">
                <h5><i class="fas fa-file-medical"></i> FHIR Resources (${resources.length})</h5>
        `;

        resources.forEach((resource, index) => {
            const resourceType = resource.resourceType || 'Unknown';
            const resourceId = resource.id || 'N/A';

            html += `
                <details class="resource-card">
                    <summary>
                        <i class="fas fa-file-medical-alt"></i> ${resourceType} (ID: ${resourceId})
                    </summary>
                    <div class="resource-card-content">
            `;

            // Render narrative HTML if available
            if (resource.text && resource.text.div) {
                html += `
                    <div class="narrative-section">
                        <h6>Human-Readable Summary</h6>
                        <div class="narrative-content">
                            ${resource.text.div}
                        </div>
                    </div>
                `;
            }

            // Show resource JSON
            html += `
                <details style="margin-top: 0.5rem;">
                    <summary style="cursor: pointer; font-size: 0.875rem; color: #64748b;">
                        <i class="fas fa-code"></i> View Full ${resourceType} JSON
                    </summary>
                    <pre style="background: #f8fafc; padding: 0.75rem; border-radius: 0.25rem; overflow: auto; max-height: 300px; font-size: 0.75rem; margin-top: 0.5rem;">${this.escapeHtml(JSON.stringify(resource, null, 2))}</pre>
                </details>
            `;

            html += `
                    </div>
                </details>
            `;
        });

        html += `</div>`;
        return html;
    }

    /**
     * Render validation errors
     */
    renderValidationErrors(errors) {
        if (!errors || errors.length === 0) return '';

        let html = `
            <div class="validation-errors-section">
                <h5>
                    <i class="fas fa-exclamation-triangle"></i> Validation Warnings (${errors.length})
                </h5>
                <ul>
        `;

        errors.forEach(error => {
            html += `<li>${this.escapeHtml(error)}</li>`;
        });

        html += `
                </ul>
            </div>
        `;

        return html;
    }

    /**
     * Copy FHIR bundle to clipboard
     */
    async copyBundleToClipboard(bundle) {
        if (!bundle) {
            this.dragDropManager.showNotification('No FHIR bundle to copy', 'warning');
            return;
        }

        try {
            await navigator.clipboard.writeText(JSON.stringify(bundle, null, 2));
            this.dragDropManager.showNotification('FHIR Bundle copied to clipboard', 'success');
        } catch (error) {
            console.error('Failed to copy:', error);
            this.dragDropManager.showNotification('Failed to copy bundle', 'error');
        }
    }

    /**
     * Copy results to clipboard
     */
    async copyResultsToClipboard(result) {
        try {
            await navigator.clipboard.writeText(JSON.stringify(result, null, 2));
            this.dragDropManager.showNotification('Full results copied to clipboard', 'success');
        } catch (error) {
            console.error('Failed to copy:', error);
            this.dragDropManager.showNotification('Failed to copy results', 'error');
        }
    }

    /**
     * Escape HTML to prevent XSS
     */
    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    /**
     * Save pipeline
     */
    async savePipeline() {
        try {
            const saveBtn = document.getElementById('savePipelineBtn');
            if (saveBtn) {
                saveBtn.disabled = true;
                saveBtn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> Saving...';
            }

            const result = await window.pipelineAPI.savePipeline(this.pipeline);

            if (result.success) {
                this.isSaved = true;
                this.updateAutoSaveStatus('saved');
                this.dragDropManager.showNotification('Pipeline saved successfully', 'success');

                // Update pipeline ID if new
                if (result.data?.id) {
                    this.pipeline.id = result.data.id;
                }
            }

        } catch (error) {
            console.error('Save failed:', error);
            this.dragDropManager.showNotification('Failed to save pipeline: ' + error.message, 'error');
        } finally {
            const saveBtn = document.getElementById('savePipelineBtn');
            if (saveBtn) {
                saveBtn.disabled = false;
                saveBtn.innerHTML = '<i class="fas fa-save"></i> Save Pipeline';
            }
        }
    }

    /**
     * Setup auto-save
     */
    setupAutoSave() {
        setInterval(() => {
            if (!this.isSaved) {
                this.autoSave();
            }
        }, 30000); // Auto-save every 30 seconds
    }

    /**
     * Auto-save pipeline
     */
    async autoSave() {
        try {
            this.updateAutoSaveStatus('saving');
            await window.pipelineAPI.savePipeline(this.pipeline);
            this.isSaved = true;
            this.updateAutoSaveStatus('saved');
        } catch (error) {
            console.error('Auto-save failed:', error);
        }
    }

    /**
     * Update auto-save status indicator
     */
    updateAutoSaveStatus(status) {
        const indicator = document.getElementById('autoSaveStatus');
        if (!indicator) return;

        if (status === 'saving') {
            indicator.textContent = 'Saving...';
            indicator.className = 'status-indicator saving';
        } else if (status === 'saved') {
            indicator.textContent = 'All changes saved';
            indicator.className = 'status-indicator saved';
        } else {
            indicator.textContent = '';
            indicator.className = 'status-indicator';
        }
    }

    /**
     * Mark as unsaved
     */
    markAsUnsaved() {
        this.isSaved = false;
        this.updateAutoSaveStatus('');
    }

    /**
     * Set execution mode
     */
    setExecutionMode(mode) {
        this.layerContainer.setExecutionMode(mode);

        // Update button states
        const parallelBtn = document.getElementById('parallelModeBtn');
        const inlineBtn = document.getElementById('inlineModeBtn');

        if (mode === 'parallel') {
            parallelBtn?.classList.add('active');
            inlineBtn?.classList.remove('active');
        } else {
            inlineBtn?.classList.add('active');
            parallelBtn?.classList.remove('active');
        }
    }

    /**
     * Clear canvas
     */
    clearCanvas() {
        if (confirm('Clear all steps from the canvas? This cannot be undone.')) {
            this.layerContainer.clearAllLayers();
            this.canvasRenderer.clearConnections();
            this.markAsUnsaved();
            this.dragDropManager.showNotification('Canvas cleared', 'info');
        }
    }

    /**
     * Navigate back to interfaces
     */
    navigateBack() {
        console.log('🔙 Navigate back clicked');
        console.log('isSaved:', this.isSaved);

        if (!this.isSaved) {
            const confirmLeave = confirm('You have unsaved changes. Are you sure you want to leave?');
            console.log('User confirmation:', confirmLeave);
            if (!confirmLeave) {
                return;
            }
        }

        console.log('Navigating to interfaces.html...');
        console.log('Referrer:', document.referrer);
        console.log('History length:', window.history.length);

        // Try to go back in history, or fall back to interfaces page
        if (window.history.length > 1 && document.referrer.includes('interfaces.html')) {
            console.log('Using history.back()');
            window.history.back();
        } else {
            console.log('Using window.location.href');
            window.location.href = '/interfaces.html';
        }
    }

    /**
     * Add step to layer (called by DragDropManager)
     */
    addStepToLayer(step, layerName) {
        this.layerContainer.addStepToLayer(step, layerName);
        this.markAsUnsaved();
    }

    /**
     * Add step to specific group
     */
    addStepToGroup(step, groupId) {
        // Find group and add step
        for (const visualLayer of Object.values(this.pipeline.layers)) {
            const group = visualLayer.getGroup(groupId);
            if (group) {
                step.sequence = this.layerContainer.getNextStepSequence(group);
                group.addStep(step);
                this.layerContainer.renderLayer(group.layer, visualLayer);
                this.canvasRenderer.redrawAllConnections();
                this.markAsUnsaved();
                return;
            }
        }
    }

    /**
     * Remove step from group
     */
    removeStepFromGroup(stepId, groupId) {
        this.layerContainer.removeStepFromGroup(stepId, groupId);
        this.markAsUnsaved();
    }

    /**
     * Move step to different layer
     */
    moveStepToLayer(stepId, groupId, sourceLayer, targetLayer) {
        this.layerContainer.moveStepToLayer(stepId, groupId, sourceLayer, targetLayer);
        this.markAsUnsaved();
    }

    /**
     * Update step
     */
    updateStep(updatedStep) {
        // Find and update step in pipeline
        for (const visualLayer of Object.values(this.pipeline.layers)) {
            for (const group of visualLayer.executionGroups) {
                const stepIndex = group.steps.findIndex(s => s.id === updatedStep.id);
                if (stepIndex !== -1) {
                    group.steps[stepIndex] = updatedStep;
                    this.markAsUnsaved();
                    return;
                }
            }
        }
    }

    /**
     * Find step in pipeline
     */
    findStep(stepId, groupId) {
        for (const visualLayer of Object.values(this.pipeline.layers)) {
            const group = visualLayer.getGroup(groupId);
            if (group) {
                return group.getStep(stepId);
            }
        }
        return null;
    }
}

// Export
if (typeof window !== 'undefined') {
    window.PipelineBuilder = PipelineBuilder;
}

if (typeof module !== 'undefined' && module.exports) {
    module.exports = PipelineBuilder;
}
