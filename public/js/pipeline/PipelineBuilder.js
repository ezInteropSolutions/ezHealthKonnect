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
        this.flowchartRenderer = null;

        // View mode state
        this.viewMode = 'flowchart'; // Default to flowchart (list disabled)

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

        // Initialize flowchart renderer - V2 (horizontal swim lanes)
        const canvasWrapper = document.getElementById('canvasWrapper');
        if (canvasWrapper) {
            // Toggle between V1 and V2 here
            const useV2 = true; // Set to true to use new horizontal layout

            if (useV2) {
                this.flowchartRenderer = new FlowchartOrchestratorV2(canvasWrapper, this);
                console.log('✅ Using Flowchart V2 (Horizontal Swim Lanes)');
            } else {
                this.flowchartRenderer = new FlowchartRenderer(canvasWrapper, this);
                console.log('✅ Using Flowchart V1 (Vertical Layout)');
            }
        }

        // Load view mode preference
        this.loadViewModePreference();

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

            // Render in current view mode (flowchart by default)
            console.log('🔄 About to switch view mode to:', this.viewMode);
            this.switchViewMode(this.viewMode, true); // Force initial render
            this.isSaved = true;

        } catch (error) {
            console.error('Failed to load pipeline:', error);
            this.pipeline = new VisualPipeline({
                name: 'New Pipeline'
            });
            this.switchViewMode(this.viewMode, true); // Force initial render
        }
    }

    /**
     * Render pipeline to canvas
     */
    renderPipeline() {
        this.layerContainer.renderCanvas();
        this.updateHeaderInfo();

        // Redraw connections after render
        setTimeout(() => {
            this.canvasRenderer.redrawAllConnections();
        }, 100);

        // If flowchart mode is active, also re-render flowchart
        if (this.viewMode === 'flowchart') {
            console.log('🔄 Re-rendering flowchart after pipeline load');
            this.renderFlowchart();
        }
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

        // View mode toggle (List vs Flowchart)
        const listViewBtn = document.getElementById('listViewBtn');
        const flowchartViewBtn = document.getElementById('flowchartViewBtn');

        if (listViewBtn) {
            listViewBtn.addEventListener('click', () => this.switchViewMode('list'));
        }

        if (flowchartViewBtn) {
            flowchartViewBtn.addEventListener('click', () => this.switchViewMode('flowchart'));
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

            // Check success field from test response
            const isSuccess = result.success === true;
            this.dragDropManager.showNotification(
                isSuccess ? 'Test passed' : 'Test failed',
                isSuccess ? 'success' : 'error'
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
        // STANDARDIZED response format:
        // - input: MessageContext (what came in)
        // - output: MessageContext (what went out)
        // - steps: Object keyed by step name { "stepName": { step_output: {...}, step_metadata: { duration_ms, success } } }
        // - success: boolean

        const stepsExecuted = result.steps ? Object.keys(result.steps).length : 0;

        // Extract final output from MessageContext
        const finalOutput = result.output?.payload || result.output || {};

        const validationErrors = result.error ? [result.error] : [];
        const validationWarnings = [];

        // Find FHIR bundle from steps
        let fhirBundle = null;
        let resourcesCreated = [];
        let mappingValidationErrors = [];

        if (result.steps) {
            // Check HL7→FHIR Transform step - using STANDARDIZED structure
            const transformStep = result.steps['hl7_fhir_transform'] || result.steps['hl7_to_fhir_transform'];
            if (transformStep?.step_output?.fhir_bundle) {
                fhirBundle = transformStep.step_output.fhir_bundle;
            }

            // Check Field Mapping step - using STANDARDIZED structure
            const mappingStep = result.steps['field_mapping'];
            if (mappingStep?.step_output) {
                resourcesCreated = mappingStep.step_output.resources_created || [];
                mappingValidationErrors = mappingStep.step_output.validation_errors || [];
            }
        }

        // Also check final output for bundle
        if (!fhirBundle && finalOutput.fhir_bundle) {
            fhirBundle = finalOutput.fhir_bundle;
        }

        // Also check if finalOutput IS a FHIR bundle
        if (!fhirBundle && finalOutput.resourceType === 'Bundle') {
            fhirBundle = finalOutput;
        }

        // Check if test passed
        const isSuccess = result.success === true;

        // Calculate total execution time from all steps - using STANDARDIZED structure
        let totalTimeMs = 0;
        if (result.steps) {
            totalTimeMs = Object.values(result.steps).reduce((sum, step) =>
                sum + (step.step_metadata?.duration_ms || step.duration_ms || 0), 0);
        }
        const executionTimeMs = totalTimeMs > 0 ? totalTimeMs : 'N/A';

        let html = `
            <div style="margin-bottom: 15px; text-align: right;">
                <button onclick="window.pipelineBuilder.runTest()" class="btn-secondary" style="padding: 8px 16px;">
                    <i class="fas fa-redo"></i> Run Test Again
                </button>
            </div>
            <div class="test-result ${isSuccess ? 'success' : 'error'}">
                <h4>
                    <i class="fas fa-${isSuccess ? 'check-circle' : 'times-circle'}"></i>
                    ${isSuccess ? 'Test Passed' : 'Test Failed'}
                </h4>
                <p><strong>Execution Time:</strong> ${executionTimeMs}ms</p>
                <p><strong>Steps Executed:</strong> ${stepsExecuted}</p>
                <p><strong>Status:</strong> ${result.status || 'unknown'}</p>
                ${result.errors?.length > 0 ? `<p class="error-message"><strong>Errors:</strong> ${result.errors.length} error(s) occurred</p>` : ''}
            </div>
        `;

        // Render per-step results with pass/fail indicators
        console.log('[TestResults] result keys:', Object.keys(result));
        console.log('[TestResults] typeof result.steps:', typeof result.steps);
        if (result.steps) {
            console.log('[TestResults] step names:', Object.keys(result.steps));
            const firstKey = Object.keys(result.steps)[0];
            if (firstKey) console.log('[TestResults] first step structure:', JSON.stringify(result.steps[firstKey], null, 2).substring(0, 500));
        } else {
            console.log('[TestResults] NO steps field! Full result keys:', JSON.stringify(result, null, 2).substring(0, 2000));
        }
        if (result.steps && Object.keys(result.steps).length > 0) {
            const stepEntries = Object.entries(result.steps);
            // Support both formats: step_metadata.success (TransformationTestController) and flat success (PipelineTestController)
            const getSuccess = (s) => s.step_metadata?.success ?? s.success ?? true;
            const failedCount = stepEntries.filter(([, s]) => getSuccess(s) === false).length;
            const passedCount = stepEntries.length - failedCount;

            html += `
                <div class="step-results-section">
                    <h4 class="step-results-header">
                        <i class="fas fa-tasks"></i> Step Results
                        <span class="step-results-summary">
                            <span class="step-count-pass">${passedCount} passed</span>
                            ${failedCount > 0 ? `<span class="step-count-fail">${failedCount} failed</span>` : ''}
                        </span>
                    </h4>
                    <div class="step-results-list">
            `;

            for (const [stepName, stepData] of stepEntries) {
                const stepSuccess = getSuccess(stepData);
                const durationMs = stepData.step_metadata?.duration_ms ?? stepData.duration_ms;
                const duration = durationMs != null ? `${durationMs}ms` : '';
                const statusClass = stepSuccess ? 'step-pass' : 'step-fail';
                const statusIcon = stepSuccess ? 'fa-check-circle' : 'fa-times-circle';
                const errorMsg = stepData.step_error || stepData.error || '';

                html += `
                    <div class="step-result-item ${statusClass}">
                        <div class="step-result-main">
                            <i class="fas ${statusIcon} step-result-icon"></i>
                            <span class="step-result-name">${this.escapeHtml(stepName)}</span>
                            ${duration ? `<span class="step-result-duration">${duration}</span>` : ''}
                        </div>
                        ${!stepSuccess && errorMsg ? `
                            <div class="step-result-error">
                                <i class="fas fa-exclamation-triangle"></i> ${this.escapeHtml(String(errorMsg))}
                            </div>
                        ` : ''}
                    </div>
                `;
            }

            html += `
                    </div>
                </div>
            `;
        }

        // Show validation warnings prominently if any
        if (validationWarnings.length > 0) {
            html += this.renderValidationWarnings(validationWarnings);
        }

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
     * Render validation warnings (pipeline continues, message accepted)
     */
    renderValidationWarnings(warnings) {
        if (!warnings || warnings.length === 0) return '';

        let html = `
            <div class="validation-warnings-section" style="background: #fffbeb; border: 1px solid #fbbf24; border-radius: 0.5rem; padding: 1rem; margin: 1rem 0;">
                <h5 style="color: #b45309; margin: 0 0 0.75rem 0; display: flex; align-items: center; gap: 0.5rem;">
                    <i class="fas fa-exclamation-triangle"></i> Validation Warnings (${warnings.length})
                    <span style="font-size: 0.75rem; font-weight: normal; color: #92400e; background: #fef3c7; padding: 0.25rem 0.5rem; border-radius: 0.25rem;">Pipeline Continued (ACK Sent)</span>
                </h5>
                <ul style="margin: 0; padding-left: 1.5rem; color: #78350f;">
        `;

        warnings.forEach(warning => {
            html += `<li style="margin-bottom: 0.5rem;">${this.escapeHtml(warning)}</li>`;
        });

        html += `
                </ul>
                <p style="margin: 0.75rem 0 0 0; font-size: 0.875rem; color: #92400e;">
                    <i class="fas fa-info-circle"></i> These warnings indicate data quality issues but did not stop processing.
                    The message was accepted (ACK) and the pipeline continued successfully.
                </p>
            </div>
        `;

        return html;
    }

    /**
     * Render validation errors (pipeline stopped, message rejected)
     */
    renderValidationErrors(errors) {
        if (!errors || errors.length === 0) return '';

        let html = `
            <div class="validation-errors-section" style="background: #fef2f2; border: 1px solid #ef4444; border-radius: 0.5rem; padding: 1rem; margin: 1rem 0;">
                <h5 style="color: #991b1b; margin: 0 0 0.75rem 0; display: flex; align-items: center; gap: 0.5rem;">
                    <i class="fas fa-times-circle"></i> Validation Errors (${errors.length})
                    <span style="font-size: 0.75rem; font-weight: normal; color: #7f1d1d; background: #fee2e2; padding: 0.25rem 0.5rem; border-radius: 0.25rem;">Pipeline Stopped (NACK Sent)</span>
                </h5>
                <ul style="margin: 0; padding-left: 1.5rem; color: #7f1d1d;">
        `;

        errors.forEach(error => {
            html += `<li style="margin-bottom: 0.5rem;">${this.escapeHtml(error)}</li>`;
        });

        html += `
                </ul>
                <p style="margin: 0.75rem 0 0 0; font-size: 0.875rem; color: #991b1b;">
                    <i class="fas fa-exclamation-circle"></i> These critical validation errors stopped pipeline execution.
                    The message was rejected (NACK) and will not be processed further.
                </p>
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
     * Synchronize branch relationships before saving
     * Updates parentConditionalStepId and branchType on target steps
     * based on conditional step configs (onTrue/onFalse route_to_step actions)
     * AND propagates branch membership through chains based on visual position
     */
    synchronizeBranchRelationships() {
        const allSteps = this.getAllSteps();
        if (!allSteps || allSteps.length === 0) return;

        console.log('[PipelineBuilder] Synchronizing branch relationships...');

        // First, clear existing branch relationships (they'll be re-established)
        allSteps.forEach(step => {
            step.parentConditionalStepId = null;
            step.branchType = null;
        });

        // PHASE 1: Mark direct routing targets from conditional steps
        allSteps.forEach(conditionalStep => {
            // Use VisualStep utility for OOP-compliant type detection
            const isLogicStep = VisualStep.isConditionalStep(conditionalStep);

            if (!isLogicStep || !conditionalStep.config || !conditionalStep.config.conditions) {
                return;
            }

            conditionalStep.config.conditions.forEach(condition => {
                // Check onTrue action
                const trueAction = condition.onTrue || condition.ifTrue;
                if (trueAction && trueAction.action === 'route_to_step' && trueAction.stepId) {
                    const targetStep = allSteps.find(s => s.id === trueAction.stepId);
                    if (targetStep) {
                        targetStep.parentConditionalStepId = conditionalStep.id;
                        targetStep.branchType = 'true';
                        console.log(`  TRUE branch (direct): ${conditionalStep.stepName} → ${targetStep.stepName}`);
                    }
                }

                // Check onFalse action
                const falseAction = condition.onFalse || condition.ifFalse;
                if (falseAction && falseAction.action === 'route_to_step' && falseAction.stepId) {
                    const targetStep = allSteps.find(s => s.id === falseAction.stepId);
                    if (targetStep) {
                        targetStep.parentConditionalStepId = conditionalStep.id;
                        targetStep.branchType = 'false';
                        console.log(`  FALSE branch (direct): ${conditionalStep.stepName} → ${targetStep.stepName}`);
                    }
                }
            });
        });

        // PHASE 2: Propagate branch membership through chains based on visual position
        // Steps that come AFTER a branch step (higher X position, same Y band) inherit branch membership
        this.propagateBranchMembership(allSteps);

        console.log('[PipelineBuilder] Branch relationships synchronized');
    }

    /**
     * Propagate branch membership through chains
     * If step A is in TRUE branch, and step B follows A (by position or sequence),
     * then B should also be in the TRUE branch
     */
    propagateBranchMembership(allSteps) {
        // Get steps with branch membership (marked in phase 1)
        let branchSteps = allSteps.filter(s => s.parentConditionalStepId && s.branchType);

        if (branchSteps.length === 0) return;

        console.log('[PipelineBuilder] Propagating branch membership...');

        const Y_TOLERANCE = 50;

        // Keep propagating until no more changes
        let changed = true;
        let iterations = 0;
        const maxIterations = 10; // Prevent infinite loops

        while (changed && iterations < maxIterations) {
            changed = false;
            iterations++;

            // Get current branch steps (may have grown from previous iteration)
            branchSteps = allSteps.filter(s => s.parentConditionalStepId && s.branchType);

            branchSteps.forEach(branchStep => {
                const branchY = branchStep.position_y || 0;
                const branchX = branchStep.position_x || 0;
                const branchSeq = branchStep.sequence || 0;
                const parentId = branchStep.parentConditionalStepId;
                const branchType = branchStep.branchType;

                // Find steps that should be in this branch:
                // 1. Same Y band and higher X position, OR
                // 2. Higher sequence number (for vertical branch chains)
                const stepsInChain = allSteps.filter(s => {
                    if (s.id === branchStep.id) return false;
                    if (s.parentConditionalStepId) return false; // Already has branch membership

                    const stepY = s.position_y || 0;
                    const stepX = s.position_x || 0;
                    const stepSeq = s.sequence || 0;

                    // Option 1: Same Y band and higher X position
                    const sameRowAndAfter = Math.abs(stepY - branchY) <= Y_TOLERANCE && stepX > branchX;

                    // Option 2: Higher sequence number AND positioned below or to the right
                    // (for vertical branch chains like FALSE branch)
                    const higherSequenceAndRelated = stepSeq > branchSeq &&
                                                     (stepX >= branchX - 100) && // Not too far left
                                                     (stepY >= branchY - Y_TOLERANCE); // Not above

                    return sameRowAndAfter || higherSequenceAndRelated;
                });

                // For each candidate, check if it's likely part of this branch
                // by verifying it's not part of a different branch (different Y region)
                stepsInChain.forEach(chainStep => {
                    // Don't propagate to steps that are clearly on a different Y region
                    // (e.g., TRUE branch is at Y=100, FALSE branch is at Y=300)
                    const stepY = chainStep.position_y || 0;
                    const isOnDifferentBranchRegion = Math.abs(stepY - branchY) > Y_TOLERANCE * 3;

                    if (!isOnDifferentBranchRegion) {
                        chainStep.parentConditionalStepId = parentId;
                        chainStep.branchType = branchType;
                        changed = true;
                        console.log(`  ${branchType.toUpperCase()} branch (propagated): ${branchStep.stepName} → ${chainStep.stepName}`);
                    }
                });
            });
        }

        console.log(`[PipelineBuilder] Branch propagation completed in ${iterations} iteration(s)`);
    }

    /**
     * Sync connections from flowchart to pipeline model
     * This ensures connections are saved to the database
     */
    syncConnectionsToPipeline() {
        if (!this.flowchartRenderer || !this.flowchartRenderer.layout) {
            console.log('[PipelineBuilder] No flowchart layout - skipping connection sync');
            return;
        }

        const connections = this.flowchartRenderer.layout.connections || [];

        // Convert connections to a serializable format
        this.pipeline.connections = connections.map(conn => ({
            from: conn.from,
            to: conn.to,
            type: conn.type || 'sequential'
        }));

        console.log(`[PipelineBuilder] Synced ${this.pipeline.connections.length} connections to pipeline model`);
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

            // Synchronize branch relationships before saving
            this.synchronizeBranchRelationships();

            // Auto-sequence steps based on connections (no manual sequence entry needed)
            if (this.flowchartRenderer && typeof this.flowchartRenderer.autoSequenceSteps === 'function') {
                console.log('🔢 Auto-sequencing steps before save...');
                this.flowchartRenderer.autoSequenceSteps(true); // silent mode
            }

            // Sync connections from flowchart to pipeline model (for database persistence)
            this.syncConnectionsToPipeline();

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
            // Synchronize branch relationships before auto-saving
            this.synchronizeBranchRelationships();
            // Auto-sequence steps based on connections
            if (this.flowchartRenderer && typeof this.flowchartRenderer.autoSequenceSteps === 'function') {
                this.flowchartRenderer.autoSequenceSteps(true); // silent mode
            }
            // Sync connections to pipeline model
            this.syncConnectionsToPipeline();
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
    async clearCanvas() {
        const confirmed = await this.dragDropManager.showConfirmDialog(
            'Clear all steps from the canvas? This cannot be undone.',
            {
                title: 'Clear Canvas',
                confirmText: 'Clear All',
                cancelText: 'Cancel',
                type: 'danger'
            }
        );

        if (confirmed) {
            this.layerContainer.clearCanvas();
            this.canvasRenderer.clearConnections();
            this.markAsUnsaved();
            this.dragDropManager.showNotification('Canvas cleared', 'info');
        }
    }

    /**
     * Navigate back to interfaces
     */
    async navigateBack() {
        console.log('🔙 Navigate back clicked');
        console.log('isSaved:', this.isSaved);

        if (!this.isSaved) {
            const confirmLeave = await this.dragDropManager.showConfirmDialog(
                'You have unsaved changes. Are you sure you want to leave?',
                {
                    title: 'Unsaved Changes',
                    confirmText: 'Leave',
                    cancelText: 'Stay',
                    type: 'warning'
                }
            );
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
     * Add step to pipeline (called by DragDropManager)
     */
    addStep(step) {
        this.layerContainer.addStep(step);
        this.markAsUnsaved();

        // In flowchart mode, trigger re-render
        if (this.viewMode === 'flowchart' && this.flowchartRenderer) {
            console.log('🔄 Triggering flowchart re-render after adding step:', step.stepName);
            const allSteps = this.getAllStepsFlat();
            this.flowchartRenderer.render(allSteps);
        }
    }

    /**
     * @deprecated Use addStep() instead
     */
    addStepToLayer(step, _layerName) {
        this.addStep(step);
    }

    /**
     * Add step to specific group
     */
    addStepToGroup(step, groupId) {
        for (const group of this.pipeline.executionGroups) {
            if (group.id === groupId || group.groupId === groupId) {
                if (!(step instanceof VisualStep)) {
                    step = new VisualStep(step);
                }
                step.sequence = this.layerContainer.getNextStepSequence(group);
                group.addStep(step);
                this.layerContainer.renderCanvas();
                this.canvasRenderer.redrawAllConnections();
                this.markAsUnsaved();
                return;
            }
        }
    }

    /**
     * Delete step from pipeline (finds the group automatically)
     */
    deleteStep(stepId) {
        console.log('🗑️ Deleting step from pipeline:', stepId);

        for (let i = 0; i < this.pipeline.executionGroups.length; i++) {
            const group = this.pipeline.executionGroups[i];
            const stepIndex = group.steps.findIndex(s => s.id === stepId);
            if (stepIndex !== -1) {
                console.log(`✅ Found step at index ${stepIndex} in group ${group.groupId || group.id}`);

                // Remove the step
                group.steps.splice(stepIndex, 1);

                // Mark as unsaved
                this.markAsUnsaved();

                // Show notification
                if (this.dragDropManager) {
                    this.dragDropManager.showNotification(`Deleted step`, 'success');
                }

                return true;
            }
        }

        console.error('❌ Step not found:', stepId);
        return false;
    }

    /**
     * Remove step from group
     */
    removeStepFromGroup(stepId, groupId) {
        this.layerContainer.removeStepFromGroup(stepId, groupId);
        this.markAsUnsaved();
    }

    /**
     * Reorder steps based on current visual order
     */
    reorderSteps() {
        let allSteps = [];
        this.pipeline.executionGroups.forEach(group => {
            group.steps.forEach(step => {
                allSteps.push({ step, group });
            });
        });

        // Update sequence numbers based on current order (increments of 10)
        allSteps.forEach((item, index) => {
            item.step.sequence = (index + 1) * 10;
        });

        this.layerContainer.renderCanvas();
        this.markAsUnsaved();
    }

    /**
     * @deprecated Use reorderSteps() instead
     */
    reorderStepsInLayer(_layerName) {
        this.reorderSteps();
    }

    /**
     * Update step
     */
    updateStep(updatedStep) {
        for (const group of this.pipeline.executionGroups) {
            const stepIndex = group.steps.findIndex(s => s.id === updatedStep.id);
            if (stepIndex !== -1) {
                if (!(updatedStep instanceof VisualStep)) {
                    updatedStep = new VisualStep(updatedStep);
                }
                group.steps[stepIndex] = updatedStep;
                this.markAsUnsaved();
                return;
            }
        }
    }

    /**
     * Find step in pipeline
     */
    findStep(stepId, groupId) {
        for (const group of this.pipeline.executionGroups) {
            if (groupId && (group.id !== groupId && group.groupId !== groupId)) continue;
            const step = group.getStep ? group.getStep(stepId) : group.steps.find(s => s.id === stepId);
            if (step) return step;
        }
        return null;
    }

    /**
     * Switch view mode (list vs flowchart)
     */
    switchViewMode(mode, force = false) {
        console.log(`🔄 switchViewMode called:`, { currentMode: this.viewMode, requestedMode: mode, force });

        if (this.viewMode === mode && !force) {
            console.log('⏭️ Already in requested mode, skipping switch');
            return;
        }

        this.viewMode = mode;

        // Update button states
        const listBtn = document.getElementById('listViewBtn');
        const flowchartBtn = document.getElementById('flowchartViewBtn');

        if (listBtn && flowchartBtn) {
            if (mode === 'list') {
                listBtn.classList.add('active');
                flowchartBtn.classList.remove('active');
            } else {
                listBtn.classList.remove('active');
                flowchartBtn.classList.add('active');
            }
        }

        // Switch canvas display
        const canvasWrapper = document.getElementById('canvasWrapper');
        const canvasLayers = document.getElementById('canvasLayers');

        if (!canvasWrapper) return;

        if (mode === 'flowchart') {
            // Hide list view
            if (canvasLayers) {
                canvasLayers.style.display = 'none';
            }

            // Add flowchart mode class
            canvasWrapper.classList.add('flowchart-mode');

            // Show flowchart canvas
            const flowchartCanvas = this.flowchartRenderer.getCanvas();
            if (flowchartCanvas && !flowchartCanvas.parentNode) {
                canvasWrapper.appendChild(flowchartCanvas);
            }

            // Ensure canvas is populated first (needed for getAllStepsFlat)
            if (this.pipeline && this.pipeline.executionGroups.length > 0) {
                this.layerContainer.renderCanvas();
            }

            // Render flowchart
            this.renderFlowchart();
        } else {
            // Hide flowchart
            const flowchartCanvas = this.flowchartRenderer.getCanvas();
            if (flowchartCanvas && flowchartCanvas.parentNode) {
                flowchartCanvas.remove();
            }

            // Remove flowchart mode class
            canvasWrapper.classList.remove('flowchart-mode');

            // Show list view
            if (canvasLayers) {
                canvasLayers.style.display = '';
            }

            // Re-render list view
            this.renderPipeline();
        }

        // Reinitialize drag-drop zones for new view mode
        if (this.dragDropManager) {
            this.dragDropManager.reinitialize();
        }

        // Save preference
        this.saveViewModePreference(mode);

        console.log(`✅ Switched to ${mode} view`);
    }

    /**
     * Render flowchart mode
     */
    renderFlowchart() {
        if (!this.flowchartRenderer) {
            console.error('❌ No flowchart renderer available');
            return;
        }

        console.log('🔍 Pipeline state:', {
            hasPipeline: !!this.pipeline,
            executionGroups: this.pipeline?.executionGroups?.length || 0
        });

        const steps = this.getAllStepsFlat();
        console.log('🎨 Rendering flowchart with steps:', steps.length, steps);

        if (steps.length === 0) {
            console.warn('⚠️ No steps to render in flowchart - pipeline may be empty');
        }

        this.flowchartRenderer.render(steps);
    }

    /**
     * Get pipeline data (for external components like IfThenElseBuilder)
     */
    getPipeline() {
        return {
            ...this.pipeline,
            steps: this.getAllStepsFlat()
        };
    }

    /**
     * Get all steps as flat array
     */
    getAllStepsFlat() {
        if (!this.pipeline) {
            return [];
        }

        // Use getAllSteps if available (from VisualPipeline)
        if (this.pipeline.getAllSteps) {
            return this.pipeline.getAllSteps();
        }

        // Fallback: iterate executionGroups directly
        const steps = [];
        const groups = this.pipeline.executionGroups || [];
        groups.forEach(group => {
            if (group.steps && Array.isArray(group.steps)) {
                steps.push(...group.steps);
            }
        });

        steps.sort((a, b) => (a.sequence || 0) - (b.sequence || 0));
        return steps;
    }

    /**
     * Alias for getAllStepsFlat (for backward compatibility)
     */
    getAllSteps() {
        return this.getAllStepsFlat();
    }

    /**
     * Load view mode preference from localStorage
     */
    loadViewModePreference() {
        const savedMode = localStorage.getItem('pipelineViewMode');
        console.log('🔍 Loading view mode preference:', { savedMode, currentViewMode: this.viewMode });

        if (savedMode === 'flowchart' || savedMode === 'list') {
            this.viewMode = savedMode;
        }

        // Always initialize with flowchart mode (hide list view from start)
        const canvasLayers = document.getElementById('canvasLayers');
        if (canvasLayers && this.viewMode === 'flowchart') {
            console.log('✅ Hiding canvasLayers during initialization');
            canvasLayers.style.display = 'none';
        }
    }

    /**
     * Save view mode preference to localStorage
     */
    saveViewModePreference(mode) {
        localStorage.setItem('pipelineViewMode', mode);
    }

    /**
     * Callback for step selection (from flowchart)
     */
    onStepSelected(stepId) {
        const allSteps = this.getAllStepsFlat();
        const step = allSteps.find(s => s.id === stepId);
        if (step) {
            this.openStepProperties(step);
        }
    }

    /**
     * Open step properties modal
     */
    openStepProperties(step) {
        if (this.propertiesPanel) {
            this.propertiesPanel.openStepModal(step);
        }
    }
}

// Export
if (typeof window !== 'undefined') {
    window.PipelineBuilder = PipelineBuilder;
}

if (typeof module !== 'undefined' && module.exports) {
    module.exports = PipelineBuilder;
}
