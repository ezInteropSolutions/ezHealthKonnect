/**
 * Properties Panel Manager
 * Manages the right panel for step configuration
 * Version: 21.4 - Redis Query Builder integration with inline HTML escaping
 */

class PropertiesPanel {
    constructor(pipelineBuilder) {
        this.builder = pipelineBuilder;
        this.container = document.getElementById('propertiesContent');
        this.currentStep = null;
    }

    /**
     * Show properties for selected step in MODAL with Tabs
     * @param {Object} step - The step to show properties for
     * @param {boolean} isPreview - If true, this is a preview (not yet added to pipeline)
     */
    showStepProperties(step, isPreview = false) {
        this.currentStep = step;
        this.isPreviewMode = isPreview;

        // Keep the AI context bar accurate as the user moves between steps.
        if (window.AIAssistant && step.id) {
            const pipeline = this.builder?.pipeline || {};
            window.AIAssistant.setContext({
                step_id:      step.id,
                pipeline_id:  pipeline.id        || '',
                interface_id: pipeline.interfaceId|| '',
                message_type: pipeline.messageType|| '',
                page:         'pipeline-builder'
            });
        }

        // Kick off async loads for the path picker so data is ready by the time
        // the user clicks a field input (both are fire-and-forget):
        // 1. Backend-declared step variables via GetOutputVariables()
        // 2. Silent test-run refresh using stored localStorage sample message
        if (!isPreview) {
            if (typeof this.initStepVariables === 'function') this.initStepVariables();
            this._silentRefreshTestOutput();
        }

        // Get modal elements
        const modal = document.getElementById('stepPropertiesModal');
        const modalTitle = document.getElementById('stepModalTitle');
        const formTabContent = document.getElementById('formTabContent');
        const variablesTabContent = document.getElementById('variablesTabContent');
        const jsonTabContent = document.getElementById('jsonTabContent');
        const docsTabContent = document.getElementById('docsTabContent');
        const aiTabContent = document.getElementById('aiTabContent');

        if (!modal || !formTabContent || !variablesTabContent || !jsonTabContent || !docsTabContent) {
            console.error('Step properties modal or tab containers not found');
            return;
        }

        // Update modal title input (now an editable field)
        if (modalTitle) {
            const prefix = isPreview ? 'Preview: ' : '';
            modalTitle.value = prefix + (step.stepName || 'Step Configuration');
            modalTitle.readOnly = isPreview;

            // Live rename: update step.stepName as the user types
            modalTitle.oninput = null; // remove previous listener
            if (!isPreview) {
                modalTitle.oninput = () => {
                    const newName = modalTitle.value.trim();
                    if (newName) step.stepName = newName;
                };
            }
        }

        // Populate Form Tab
        formTabContent.innerHTML = '';
        const formUI = this.createFormUI(step, isPreview);
        formTabContent.appendChild(formUI);

        // Populate Variables Tab (Reference Variables)
        this.setupVariablesTab(step, variablesTabContent);

        // Populate JSON Tab
        jsonTabContent.innerHTML = '';
        const jsonUI = this.createJSONEditor(step, isPreview);
        jsonTabContent.appendChild(jsonUI);

        // Populate Documentation Tab
        docsTabContent.innerHTML = '';
        const docsUI = this.createDocumentation(step);
        docsTabContent.appendChild(docsUI);

        // Populate AI Tab
        if (aiTabContent) {
            aiTabContent.innerHTML = '';
            this.setupAITab(step, aiTabContent, isPreview);
        }

        // Setup tab switching
        this.setupTabSwitching(modal);

        // Setup fullscreen toggle
        this.setupFullscreenToggle(modal);

        // Show modal
        modal.style.display = 'flex';

        // Setup modal close handlers
        this.setupModalCloseHandlers(modal, isPreview);

    }

    /**
     * Setup modal close handlers
     */
    setupModalCloseHandlers(modal, isPreview = false) {
        // Close on X button
        const closeBtn = modal.querySelector('.modal-close');
        if (closeBtn) {
            closeBtn.onclick = () => {
                this.closeModal();
            };
        }

        // Close on overlay click
        modal.onclick = (e) => {
            if (e.target === modal) {
                this.closeModal();
            }
        };

        // Close on ESC key
        const escHandler = (e) => {
            if (e.key === 'Escape' && modal.style.display === 'flex') {
                this.closeModal();
                document.removeEventListener('keydown', escHandler);
            }
        };
        document.addEventListener('keydown', escHandler);
    }

    /**
     * Setup Variables Tab - Shows available reference variables for this step
     */
    setupVariablesTab(step, container) {
        // Find step's position in pipeline to determine available variables
        const { layerName, stepIndex } = this.findStepPosition(step);

        console.log('📚 Variables Tab - Step position:', {
            stepName: step.stepName,
            stepId: step.id,
            layerName,
            stepIndex
        });

        // Initialize reference variables panel if not already created
        if (!window.referencePanel) {
            window.referencePanel = new ReferenceVariablesPanel(container, this.builder);
        } else {
            window.referencePanel.container = container;
        }

        // Show variables for this step
        if (layerName && stepIndex !== -1) {
            console.log('✅ Showing variables for step in layer:', layerName, 'index:', stepIndex);
            window.referencePanel.show(step, layerName, stepIndex);
        } else {
            console.log('⚠️  Step not found in pipeline - showing preview message');
            // Step not yet added to pipeline (preview mode)
            container.innerHTML = `
                <div style="display: flex; flex-direction: column; align-items: center; justify-content: center; padding: 48px; text-align: center;">
                    <div style="font-size: 48px; opacity: 0.3; margin-bottom: 16px;">ℹ️</div>
                    <p style="color: #6b7280; margin: 0;">Add this step to the pipeline first</p>
                    <p style="color: #9ca3af; font-size: 13px; margin: 8px 0 0 0;">
                        Variables will be available after the step is added
                    </p>
                </div>
            `;
        }

        // For script steps, append a "Code Template Functions" accordion
        const scriptStepTypes = ['enrichment.script', 'pre.enrichment.script'];
        if (scriptStepTypes.includes(step.stepType)) {
            this.appendCodeTemplateFunctionsPanel(container, step);
        }
    }

    /**
     * Appends an "Available Functions (Code Templates)" accordion to the Variables tab.
     * Shows functions injected into the goja VM for the current interface's script steps.
     */
    appendCodeTemplateFunctionsPanel(container, step) {
        const interfaceId = this.builder?.pipeline?.interfaceId || '';
        const url = interfaceId
            ? `/api/code-templates/for-interface/${interfaceId}`
            : '/api/code-templates?scope=global&is_active=true';

        const accordion = document.createElement('div');
        accordion.style.cssText = 'border-top:1px solid #e5e7eb;margin-top:16px;';
        accordion.innerHTML = `
            <div style="padding:10px 16px;display:flex;align-items:center;gap:8px;cursor:pointer;user-select:none;background:#f8fafc;"
                 onclick="this.nextElementSibling.style.display=this.nextElementSibling.style.display==='none'?'':'none';this.querySelector('.ct-toggle').textContent=this.nextElementSibling.style.display===''?'▾':'▸'">
                <i class="fas fa-code" style="color:#1e3a8a;font-size:13px;"></i>
                <span style="font-size:12px;font-weight:700;color:#1e3a8a;">Available Functions <span style="font-weight:400;color:#6b7280;">(from Code Templates)</span></span>
                <span class="ct-toggle" style="margin-left:auto;color:#6b7280;font-size:11px;">▾</span>
            </div>
            <div class="ct-fn-body" style="padding:12px 16px;background:#fff;"></div>`;
        container.appendChild(accordion);

        const body = accordion.querySelector('.ct-fn-body');
        body.innerHTML = '<span style="font-size:12px;color:#9ca3af;">Loading…</span>';

        fetch(url)
            .then(r => r.json())
            .then(json => {
                const templates = json.data || [];
                if (templates.length === 0) {
                    body.innerHTML = '<span style="font-size:12px;color:#9ca3af;">No code templates active for this interface.</span>';
                    return;
                }
                body.innerHTML = '';
                templates.forEach(t => {
                    const sigs = t.function_signatures || [];
                    if (sigs.length === 0) return;
                    const group = document.createElement('div');
                    group.style.cssText = 'margin-bottom:10px;';
                    group.innerHTML = `<div style="font-size:11px;font-weight:700;color:#374151;margin-bottom:5px;font-family:monospace;">${t.name}</div>`;
                    sigs.forEach(sig => {
                        const chip = document.createElement('div');
                        chip.style.cssText = 'display:inline-flex;align-items:center;gap:4px;font-size:11px;font-family:monospace;padding:3px 8px;background:#f8fafc;border:1px solid #e2e8f0;border-radius:4px;color:#334155;cursor:pointer;margin:2px 3px 2px 0;';
                        chip.title = 'Click to insert at cursor';
                        chip.textContent = sig;
                        chip.addEventListener('click', () => {
                            const fnName = sig.split('(')[0];
                            this._insertFunctionAtCursor(fnName + '(');
                        });
                        group.appendChild(chip);
                    });
                    body.appendChild(group);
                });
            })
            .catch(() => {
                body.innerHTML = '<span style="font-size:12px;color:#ef4444;">Could not load code templates.</span>';
            });
    }

    /**
     * Inserts text at the cursor position in the script textarea.
     */
    _insertFunctionAtCursor(text) {
        const textarea = document.querySelector('#scriptEnrichmentEditorContainer textarea, [id*="script"] textarea, textarea[id*="Script"]');
        if (!textarea) return;
        textarea.focus();
        const start = textarea.selectionStart;
        const end   = textarea.selectionEnd;
        textarea.value = textarea.value.slice(0, start) + text + textarea.value.slice(end);
        textarea.selectionStart = textarea.selectionEnd = start + text.length;
        textarea.dispatchEvent(new Event('input'));
    }

    /**
     * Find step's position (flat sequential index) across all execution groups.
     * Returns { layerName, stepIndex } — layerName is the group name/id (truthy when found).
     */
    findStepPosition(step) {
        const pipeline = this.builder.pipeline;
        if (!pipeline || !pipeline.executionGroups) {
            return { layerName: null, stepIndex: -1 };
        }

        let flatIndex = 0;
        for (const group of pipeline.executionGroups) {
            if (!group.steps) continue;
            for (let i = 0; i < group.steps.length; i++) {
                const s = group.steps[i];
                if (s.id === step.id ||
                    (s.stepName === step.stepName && s.stepType === step.stepType)) {
                    return {
                        layerName: group.name || group.id || 'main',
                        stepIndex: flatIndex,
                    };
                }
                flatIndex++;
            }
        }
        return { layerName: null, stepIndex: -1 };
    }

    /**
     * Setup tab switching functionality
     */
    setupTabSwitching(modal) {
        const tabs = modal.querySelectorAll('.modal-tab');
        const tabContents = modal.querySelectorAll('.tab-content');

        tabs.forEach(tab => {
            tab.addEventListener('click', () => {
                // Remove active from all tabs
                tabs.forEach(t => {
                    t.classList.remove('active');
                    t.style.background = 'transparent';
                    t.style.color = '#6b7280';
                    t.style.borderBottomColor = 'transparent';
                });

                // Remove active from all tab contents
                tabContents.forEach(tc => {
                    tc.classList.remove('active');
                    tc.style.display = 'none';
                });

                // Add active to clicked tab
                tab.classList.add('active');
                tab.style.background = 'white';
                tab.style.color = '#1e3a8a';
                tab.style.borderBottomColor = '#f8bbd9'; // Pastel pink accent

                // Show corresponding content
                const tabName = tab.getAttribute('data-tab');
                const targetContent = document.getElementById(`${tabName}TabContent`);
                if (targetContent) {
                    targetContent.classList.add('active');
                    targetContent.style.display = 'block';
                }
            });
        });
    }

    /**
     * Setup fullscreen toggle functionality
     */
    setupFullscreenToggle(modal) {
        const fullscreenBtn = modal.querySelector('.modal-fullscreen-btn');
        if (!fullscreenBtn) return;

        const icon = fullscreenBtn.querySelector('i');
        let isFullscreen = false;

        fullscreenBtn.addEventListener('click', () => {
            isFullscreen = !isFullscreen;

            if (isFullscreen) {
                // Enter fullscreen
                modal.classList.add('fullscreen');
                icon.className = 'fas fa-compress';
                fullscreenBtn.title = 'Exit Fullscreen';
                console.log('✅ Entered fullscreen mode');
            } else {
                // Exit fullscreen
                modal.classList.remove('fullscreen');
                icon.className = 'fas fa-expand';
                fullscreenBtn.title = 'Toggle Fullscreen';
                console.log('✅ Exited fullscreen mode');
            }
        });

        // Also support F11 key for fullscreen toggle
        const f11Handler = (e) => {
            if (e.key === 'F11' && modal.style.display === 'flex') {
                e.preventDefault();
                fullscreenBtn.click();
            }
        };
        document.addEventListener('keydown', f11Handler);

        // Remove handler when modal closes
        const originalCloseModal = this.closeModal.bind(this);
        this.closeModal = () => {
            document.removeEventListener('keydown', f11Handler);
            modal.classList.remove('fullscreen');
            isFullscreen = false;
            icon.className = 'fas fa-expand';
            originalCloseModal();
        };
    }

    /**
     * Create Form UI (user-friendly form interface)
     */
    createFormUI(step, isPreview = false) {
        const form = document.createElement('div');
        form.className = 'properties-form';

        // SPECIAL HANDLING: Script Enrichment uses new beautiful editor
        // Use VisualStep utility for type detection (handles old and new type names)
        if (VisualStep.isScriptEnrichment(step)) {
            form.innerHTML = '<div id="scriptEnrichmentEditorContainer"></div>';

            // Find the step's position in the pipeline to show correct variables
            const stepPosition = this.findStepPosition(step);
            console.log('📍 Script editor - step position:', stepPosition);

            // Initialize the beautiful script editor after DOM insertion
            setTimeout(() => {
                const container = document.getElementById('scriptEnrichmentEditorContainer');
                if (container && typeof ScriptEnrichmentEditor !== 'undefined') {
                    new ScriptEnrichmentEditor('scriptEnrichmentEditorContainer', {
                        pipelineId: this.builder.pipeline?.id,
                        pipelineBuilder: this.builder, // Pass the builder instance for ReferenceVariablesPanel
                        stepName: step.stepName,
                        stepConfig: step.config || {},
                        stepIndex: stepPosition.stepIndex,
                        onSave: (config) => {
                            step.config = config;
                            this.saveStep(step, isPreview);
                        },
                        onCancel: () => {
                            this.closeModal();
                        },
                        onChange: (config) => {
                            step.config = config;
                        }
                    });
                }
            }, 100);

            return form;
        }

        // DEFAULT HANDLING: Regular form for other step types
        const actionButtons = isPreview ? `
            <div class="form-actions" style="margin-top: 1.5rem; display: flex; gap: 0.5rem;">
                <button class="btn btn-primary" id="addToPipelineBtn">
                    <i class="fas fa-plus"></i> Add to Pipeline
                </button>
                <button class="btn btn-secondary" id="cancelStepBtn">
                    <i class="fas fa-times"></i> Close
                </button>
            </div>
        ` : `
            <div class="form-actions" style="margin-top: 1.5rem; display: flex; gap: 0.5rem;">
                <button class="btn btn-primary" id="saveStepBtn">
                    <i class="fas fa-save"></i> Save
                </button>
                <button class="btn btn-secondary" id="cancelStepBtn">
                    <i class="fas fa-times"></i> Cancel
                </button>
            </div>
        `;

        form.innerHTML = `
            <h3 style="margin-bottom: 1rem; display: flex; align-items: center; gap: 0.5rem;">
                <i class="${step.icon}"></i>
                <span>Form Configuration</span>
            </h3>

            ${this.createBasicPropertiesSection(step)}
            ${this.createExecutionPropertiesSection(step)}
            ${this.createDynamicFormFields(step)}
            ${this.createErrorHandlingSection(step)}
            ${step.stepType === 'custom' || step.scriptContent ? this.createScriptSection(step) : ''}

            ${actionButtons}
        `;

        // Attach event listeners
        this.attachFormEvents(form, step, isPreview);

        return form;
    }

    /**
     * Returns a step-type-aware JSON placeholder shown in the Import JSON textarea
     * after the user clears it. Each placeholder is a complete, runnable example.
     */
    _getJSONPlaceholder(stepType) {
        const examples = {
            remove_duplicates: JSON.stringify({
                stepName: 'Remove Duplicate Patients',
                stepType: 'remove_duplicates',
                sequence: 30,
                enabled: true,
                config: {
                    sourceField: 'steps.file_parser_a1b2c3.step_output.records',
                    keyFields: ['patient_id', 'visit_date'],
                    strategy: 'first',
                    caseSensitive: false,
                    nullKeyBehavior: 'remove',
                    outputField: ''
                }
            }, null, 2),
            file_parser: JSON.stringify({
                stepName: 'Parse Claims CSV',
                stepType: 'file_parser',
                sequence: 10,
                enabled: true,
                config: {
                    sourceType: 'local_path',
                    filePath: '/data/claims/claims_export.csv',
                    fileFormat: 'csv',
                    hasHeader: true,
                    delimiter: ',',
                    trimFields: true,
                    maxRecords: 10000,
                    offset: 0,
                    skipRows: 0
                }
            }, null, 2),
            'enrichment.api': JSON.stringify({
                stepName: 'Enrich from EMPI',
                stepType: 'enrichment.api',
                sequence: 20,
                enabled: true,
                config: {
                    endpoint: 'https://empi.hospital.org/patients/{patientId}',
                    method: 'GET',
                    authType: 'bearer',
                    fieldMappings: { patientId: 'PID.3' },
                    timeoutMs: 5000,
                    failOnError: false
                }
            }, null, 2),
            'enrichment.database': JSON.stringify({
                stepName: 'Lookup Insurance Plan',
                stepType: 'enrichment.database',
                sequence: 20,
                enabled: true,
                config: {
                    databaseType: 'postgresql',
                    query: 'SELECT plan_name, group_id, effective_date FROM insurance_plans WHERE member_id = $1',
                    queryParams: { memberId: 'IN1.36' },
                    cacheResults: true,
                    cacheTTL: 300,
                    failOnError: false
                }
            }, null, 2),
            data_masking: JSON.stringify({
                stepName: 'Mask PHI Fields',
                stepType: 'data_masking',
                sequence: 200,
                enabled: true,
                config: {
                    maskAllPHI: false,
                    maskAllPHIFormat: 'hl7v2',
                    preserveFormat: false,
                    rules: [
                        {
                            field: 'PID.5',
                            strategy: 'mask',
                            comment: 'Patient name — full mask (HL7 v2 path)'
                        },
                        {
                            field: 'PID.19',
                            strategy: 'hash',
                            hashSalt: 'myOrgSecret2025',
                            comment: 'SSN — SHA-256 hash, same salt = same hash across datasets (join-safe)'
                        },
                        {
                            field: 'PID.13',
                            strategy: 'partial',
                            keepFirst: 0,
                            keepLast: 4,
                            preserveFormat: true,
                            comment: 'Phone — keep last 4 digits, preserve dashes: 555-867-5309 → ***-***-5309'
                        },
                        {
                            field: 'PID.5',
                            strategy: 'substitute',
                            substituteType: 'name',
                            comment: 'Replace with realistic fake name (deterministic — same input = same fake output)'
                        },
                        {
                            field: 'steps.hl7_fhir_transform.step_output.fhir_bundle.entry[0].resource.patient.name[0].family',
                            strategy: 'mask',
                            comment: 'FHIR Patient family name from prior HL7→FHIR transform step (entry index auto-resolved)'
                        },
                        {
                            field: 'steps.api_enrichment.step_output.email',
                            strategy: 'mask',
                            comment: 'Email returned by a prior API enrichment step'
                        }
                    ]
                }
            }, null, 2),
            'data_masking_fhir': JSON.stringify({
                stepName: 'Mask FHIR PHI',
                stepType: 'data_masking',
                sequence: 200,
                enabled: true,
                config: {
                    maskAllPHI: true,
                    maskAllPHIFormat: 'fhir',
                    preserveFormat: false,
                    rules: [
                        {
                            field: 'steps.hl7_fhir_transform.step_output.fhir_bundle.entry[0].resource.patient.name[0].family',
                            strategy: 'substitute',
                            substituteType: 'name',
                            comment: 'Replace FHIR Patient family name with deterministic fake name'
                        }
                    ]
                }
            }, null, 2),
            'data_masking_csv': JSON.stringify({
                stepName: 'Mask CSV Patient Records',
                stepType: 'data_masking',
                sequence: 200,
                enabled: true,
                config: {
                    maskAllPHI: false,
                    preserveFormat: false,
                    rules: [
                        {
                            field: 'steps.file_parser.step_output.records[0].ssn',
                            strategy: 'hash',
                            hashSalt: 'myOrgSecret2025',
                            comment: 'SSN in CSV records — index [0] auto-resolved to search all records'
                        },
                        {
                            field: 'steps.file_parser.step_output.records[0].patient_name',
                            strategy: 'substitute',
                            substituteType: 'name',
                            comment: 'Replace name in every matching CSV row'
                        }
                    ]
                }
            }, null, 2),
        };
        return examples[stepType] || JSON.stringify({
            stepName: 'My Step',
            stepType: stepType || 'field_mapping',
            sequence: 10,
            enabled: true,
            config: {}
        }, null, 2);
    }

    /**
     * Create JSON Import/Export Interface
     */
    createJSONEditor(step, isPreview = false) {
        const container = document.createElement('div');
        container.className = 'json-editor-container';

        // Export current configuration as JSON
        const currentConfig = {
            stepName: step.stepName,
            stepType: step.stepType,
            sequence: step.sequence,
            enabled: step.enabled,
            config: step.config || {},
            scriptContent: step.scriptContent || ''
        };

        const formattedJSON = JSON.stringify(currentConfig, null, 2);

        // Step-type-aware placeholder shown after the user clears the textarea
        const jsonPlaceholder = this._getJSONPlaceholder(step.stepType || '');

        container.innerHTML = `
            <div class="json-editor-wrapper">

                <!-- Header Section -->
                <div class="json-editor-header">
                    <div class="header-content">
                        <h3>
                            <i class="fas fa-code"></i>
                            <span>JSON Configuration</span>
                        </h3>
                        <p class="header-subtitle">Import or export step configuration as JSON</p>
                    </div>
                </div>

                <!-- Quick Actions Card -->
                <div class="json-quick-actions">
                    <button class="quick-action-btn" id="formatJsonBtn" title="Format JSON">
                        <i class="fas fa-align-left"></i>
                        <span>Format</span>
                    </button>
                    <button class="quick-action-btn" id="validateJsonBtn" title="Validate JSON">
                        <i class="fas fa-check-circle"></i>
                        <span>Validate</span>
                    </button>
                    <button class="quick-action-btn" id="copyJsonBtn" title="Copy to Clipboard">
                        <i class="fas fa-copy"></i>
                        <span>Copy</span>
                    </button>
                    <button class="quick-action-btn" id="clearJsonBtn" title="Clear JSON">
                        <i class="fas fa-eraser"></i>
                        <span>Clear</span>
                    </button>
                </div>

                <!-- Validation Status -->
                <div id="jsonValidationStatus" class="json-validation-status" style="display: none;">
                    <i class="fas fa-check-circle"></i>
                    <span>Valid JSON</span>
                </div>

                <!-- Editor Section -->
                <div class="json-editor-section">
                    <div class="editor-label">
                        <label for="jsonConfigInput">
                            <i class="fas fa-file-code"></i> Configuration JSON
                        </label>
                        <span class="editor-hint">Paste or edit JSON configuration below</span>
                    </div>

                    <div class="json-textarea-wrapper">
                        <textarea
                            id="jsonConfigInput"
                            class="json-textarea"
                            rows="16"
                            placeholder="${jsonPlaceholder.replace(/"/g, '&quot;')}"
                            spellcheck="false"
                        >${formattedJSON}</textarea>
                    </div>
                </div>

                <!-- Info Cards -->
                <div class="json-info-cards">
                    <div class="info-card info-card-import">
                        <div class="info-card-icon">
                            <i class="fas fa-file-import"></i>
                        </div>
                        <div class="info-card-content">
                            <h4>Import Configuration</h4>
                            <p>Paste JSON from another step or external source to quickly configure this step</p>
                        </div>
                    </div>

                    <div class="info-card info-card-export">
                        <div class="info-card-icon">
                            <i class="fas fa-file-download"></i>
                        </div>
                        <div class="info-card-content">
                            <h4>Export Configuration</h4>
                            <p>Download current settings for backup, sharing, or reuse in other pipelines</p>
                        </div>
                    </div>
                </div>

                <!-- Action Buttons -->
                <div class="json-actions">
                    ${isPreview ? `
                        <button class="btn btn-primary btn-large" id="importJsonBtn">
                            <i class="fas fa-file-import"></i>
                            <span>Import & Add to Pipeline</span>
                        </button>
                        <button class="btn btn-secondary" id="cancelStepBtn">
                            <i class="fas fa-times"></i>
                            <span>Close</span>
                        </button>
                    ` : `
                        <button class="btn btn-primary btn-large" id="importJsonBtn">
                            <i class="fas fa-file-import"></i>
                            <span>Import & Update</span>
                        </button>
                        <button class="btn btn-secondary" id="exportJsonBtn">
                            <i class="fas fa-file-download"></i>
                            <span>Export Current</span>
                        </button>
                        <button class="btn btn-secondary" id="cancelStepBtn">
                            <i class="fas fa-times"></i>
                            <span>Cancel</span>
                        </button>
                    `}
                </div>
            </div>
        `;

        // Attach event listeners
        this.attachJSONImportEvents(container, step, isPreview);

        return container;
    }

    /**
     * Format example for display - handles special cases like script enrichment
     */
    formatExampleForDisplay(example, stepType) {
        // Special handling for script enrichment - display script as raw code
        if ((stepType === 'enrichment.script' || stepType === 'pre.enrichment.script') && example.script) {
            // Create a readable format showing the script content directly
            const exampleCopy = { ...example };
            const scriptContent = exampleCopy.script;
            delete exampleCopy.script;

            // Show script separately in a more readable format
            return `{
  "script": \`${scriptContent}\`,
  "timeout_ms": ${exampleCopy.timeout_ms || 5000},
  "failOnError": ${exampleCopy.failOnError || false}
}`;
        }

        // For field mapping with enriched data examples, show without escaping
        if (stepType === 'core.mapping' && example.mappings) {
            // Format mappings in a readable way
            const formatted = {
                ...example,
                mappings: example.mappings.map(m => ({
                    ...m,
                    // Show comment if exists
                    ...(m.comment && { comment: m.comment })
                }))
            };
            return JSON.stringify(formatted, null, 2);
        }

        // Default: standard JSON stringification
        return JSON.stringify(example, null, 2);
    }

    /**
     * Create Documentation Tab
     */
    createDocumentation(step) {
        const container = document.createElement('div');
        container.className = 'documentation-container';

        const docs = this.getStepDocumentation(step.stepType);

        container.innerHTML = `
            <div style="padding: 1rem;">
                <h3 style="margin-bottom: 1rem; color: #1e3a8a;">
                    <i class="${step.icon}"></i> ${step.stepName}
                </h3>

                <div class="doc-section" style="margin-bottom: 1.5rem;">
                    <h4 style="color: #2563eb; margin-bottom: 0.5rem;">
                        <i class="fas fa-info-circle"></i> Description
                    </h4>
                    <p style="color: #4b5563; line-height: 1.6;">${docs.description}</p>
                </div>

                <div class="doc-section" style="margin-bottom: 1.5rem;">
                    <h4 style="color: #2563eb; margin-bottom: 0.5rem;">
                        <i class="fas fa-lightbulb"></i> Use Cases
                    </h4>
                    <ul style="color: #4b5563; line-height: 1.8; padding-left: 1.5rem;">
                        ${docs.useCases.map(uc => `<li>${uc}</li>`).join('')}
                    </ul>
                </div>

                <div class="doc-section" style="margin-bottom: 1.5rem;">
                    <h4 style="color: #2563eb; margin-bottom: 0.5rem;">
                        <i class="fas fa-code"></i> Example Configuration
                    </h4>
                    <pre style="background: #f3f4f6; padding: 1rem; border-radius: 6px; overflow-x: auto; font-size: 0.875rem;"><code>${this.formatExampleForDisplay(docs.example, step.stepType)}</code></pre>
                </div>

                ${docs.examples ? `
                <div class="doc-section" style="margin-bottom: 1.5rem;">
                    <h4 style="color: #2563eb; margin-bottom: 0.5rem;">
                        <i class="fas fa-copy"></i> Examples
                    </h4>
                    <p style="color: #6b7280; font-size: 0.875rem; margin-bottom: 1rem;">Click an example to expand the full configuration.</p>
                    ${docs.examples.map(ex => `
                        <details style="margin-bottom: 0.75rem; border: 1px solid #e5e7eb; border-radius: 6px; background: #f9fafb;">
                            <summary style="padding: 0.75rem; cursor: pointer; font-weight: 600; color: #1e3a8a; display: flex; align-items: center; gap: 0.5rem;">
                                <i class="fas fa-chevron-right" style="font-size: 0.75rem;"></i>
                                ${ex.label}
                            </summary>
                            <div style="padding: 1rem; background: white; border-top: 1px solid #e5e7eb;">
                                ${ex.description ? `<p style="color:#4b5563;font-size:0.85rem;margin-bottom:0.75rem;">${ex.description}</p>` : ''}
                                <pre style="background: #f3f4f6; padding: 0.75rem; border-radius: 4px; font-size: 0.8rem; overflow-x: auto; line-height: 1.6;"><code>${JSON.stringify(ex.config, null, 2).replace(/\\n/g, '\n').replace(/\\t/g, '  ')}</code></pre>
                            </div>
                        </details>
                    `).join('')}
                </div>
                ` : ''}

                ${docs.connectorTypeCards ? `
                <div class="doc-section" style="margin-bottom: 1.5rem;" id="connectorTypeDocsSection">
                    <h4 style="color: #2563eb; margin-bottom: 0.75rem;">
                        <i class="fas fa-plug"></i> Connector Type Reference
                    </h4>
                    <select id="connectorTypeDocSelect" style="width:100%;padding:0.5rem;border:1px solid #d1d5db;border-radius:6px;font-size:0.875rem;margin-bottom:0.75rem;background:#fff;">
                        <option value="">— Select a connector type to view its documentation —</option>
                        ${docs.connectorTypeCards.map(ct => `<option value="${ct.typeName}">${ct.icon} ${ct.displayName} (${ct.typeName})</option>`).join('')}
                    </select>
                    <div id="connectorTypeDocDetail" style="display:none;"></div>
                </div>
                ` : ''}

                ${docs.validationTypes ? `
                <div class="doc-section" style="margin-bottom: 1.5rem;">
                    <h4 style="color: #2563eb; margin-bottom: 0.5rem;">
                        <i class="fas fa-check-circle"></i> Validation Types
                    </h4>
                    ${docs.validationTypes.map(vt => `
                        <div style="margin-bottom: 1rem; padding: 1rem; background: #f9fafb; border-left: 4px solid #3b82f6; border-radius: 4px;">
                            <div style="font-weight: 600; color: #1e3a8a; margin-bottom: 0.5rem;">
                                <code style="background: #e0e7ff; padding: 0.25rem 0.5rem; border-radius: 3px;">${vt.type}</code>
                            </div>
                            <p style="color: #4b5563; margin-bottom: 0.5rem;">${vt.description}</p>
                            <p style="color: #6b7280; font-size: 0.875rem; margin-bottom: 0.5rem;"><strong>Use For:</strong> ${vt.usedFor}</p>
                            <details style="margin-top: 0.5rem;">
                                <summary style="cursor: pointer; color: #2563eb; font-size: 0.875rem;">View Example</summary>
                                <pre style="background: #fff; padding: 0.75rem; border-radius: 4px; margin-top: 0.5rem; font-size: 0.8rem; overflow-x: auto;"><code>${JSON.stringify(vt.example, null, 2)}</code></pre>
                            </details>
                        </div>
                    `).join('')}
                </div>
                ` : ''}

                ${docs.fieldExamples ? `
                <div class="doc-section" style="margin-bottom: 1.5rem;">
                    <h4 style="color: #2563eb; margin-bottom: 0.5rem;">
                        <i class="fas fa-list"></i> Common Field Paths
                    </h4>
                    <table style="width: 100%; border-collapse: collapse; font-size: 0.875rem;">
                        <thead>
                            <tr style="background: #f3f4f6;">
                                <th style="padding: 0.6rem; text-align: left; border: 1px solid #e5e7eb;">HL7 Field</th>
                                <th style="padding: 0.6rem; text-align: left; border: 1px solid #e5e7eb;">Description</th>
                                <th style="padding: 0.6rem; text-align: left; border: 1px solid #e5e7eb;">JSONPath</th>
                            </tr>
                        </thead>
                        <tbody>
                            ${docs.fieldExamples.map(f => `
                                <tr>
                                    <td style="padding: 0.6rem; border: 1px solid #e5e7eb;"><code style="background: #e0e7ff; padding: 0.2rem 0.4rem; border-radius: 3px;">${f.field}</code></td>
                                    <td style="padding: 0.6rem; border: 1px solid #e5e7eb;">${f.description}</td>
                                    <td style="padding: 0.6rem; border: 1px solid #e5e7eb; font-family: monospace; font-size: 0.75rem; color: #059669;">${f.path}</td>
                                </tr>
                            `).join('')}
                        </tbody>
                    </table>
                </div>
                ` : ''}

                ${docs.databaseConfigs ? `
                <div class="doc-section" style="margin-bottom: 1.5rem;">
                    <h4 style="color: #2563eb; margin-bottom: 0.5rem;">
                        <i class="fas fa-database"></i> Database-Specific Configuration
                    </h4>
                    <p style="color: #6b7280; font-size: 0.875rem; margin-bottom: 1rem;">Click a database to see connection string format, query syntax, and special features.</p>
                    ${Object.keys(docs.databaseConfigs).map(dbType => {
                        const db = docs.databaseConfigs[dbType];
                        return `
                        <details style="margin-bottom: 0.75rem; border: 1px solid #e5e7eb; border-radius: 6px; background: #f9fafb;">
                            <summary style="padding: 0.75rem; cursor: pointer; font-weight: 600; color: #1e3a8a; display: flex; align-items: center; gap: 0.5rem;">
                                <i class="fas fa-chevron-right" style="font-size: 0.75rem; transition: transform 0.2s;"></i>
                                ${db.name}
                            </summary>
                            <div style="padding: 1rem; background: white; border-top: 1px solid #e5e7eb;">
                                <div style="margin-bottom: 1rem;">
                                    <strong style="color: #4b5563;">Connection String Format:</strong>
                                    <pre style="background: #f3f4f6; padding: 0.5rem; border-radius: 4px; margin-top: 0.5rem; font-size: 0.8rem; overflow-x: auto;"><code>${db.connectionFormat}</code></pre>
                                    <pre style="background: #e0e7ff; padding: 0.5rem; border-radius: 4px; margin-top: 0.5rem; font-size: 0.8rem; overflow-x: auto; color: #1e3a8a;"><code>${db.example}</code></pre>
                                </div>
                                ${db.queryFormat ? `
                                <div style="margin-bottom: 1rem;">
                                    <strong style="color: #4b5563;">Query Parameter Format:</strong>
                                    <p style="color: #6b7280; font-size: 0.875rem; margin-top: 0.5rem;">${db.queryFormat}</p>
                                    <pre style="background: #f3f4f6; padding: 0.5rem; border-radius: 4px; margin-top: 0.5rem; font-size: 0.8rem; overflow-x: auto;"><code>${db.queryExample}</code></pre>
                                </div>
                                ` : ''}
                                ${db.features && db.features.length > 0 ? `
                                <div>
                                    <strong style="color: #4b5563;">Special Features:</strong>
                                    <ul style="margin-top: 0.5rem; padding-left: 1.5rem; color: #6b7280; font-size: 0.875rem;">
                                        ${db.features.map(f => `<li>${f}</li>`).join('')}
                                    </ul>
                                </div>
                                ` : ''}
                            </div>
                        </details>
                        `;
                    }).join('')}
                </div>
                ` : ''}

                ${docs.noCodeFeatures ? `
                <div class="doc-section" style="margin-bottom: 1.5rem;">
                    <h4 style="color: #2563eb; margin-bottom: 0.5rem;">
                        <i class="fas fa-magic"></i> NO-CODE Features
                    </h4>
                    ${docs.noCodeFeatures.map(feature => `
                        <div style="margin-bottom: 1rem; padding: 1rem; background: linear-gradient(135deg, #667eea15 0%, #764ba215 100%); border-left: 4px solid #667eea; border-radius: 4px;">
                            <div style="font-weight: 600; color: #1e3a8a; margin-bottom: 0.5rem;">
                                ${feature.feature}
                            </div>
                            <p style="color: #4b5563; margin-bottom: 0.5rem; font-size: 0.875rem;">${feature.description}</p>
                            <p style="color: #6b7280; font-size: 0.875rem; margin-bottom: 0.5rem;"><strong>How:</strong> ${feature.howTo}</p>
                            <p style="color: #059669; font-size: 0.875rem;"><strong>Benefit:</strong> ${feature.benefit}</p>
                        </div>
                    `).join('')}
                </div>
                ` : ''}

                ${docs.workflow ? `
                <div class="doc-section" style="margin-bottom: 1.5rem;">
                    <h4 style="color: #2563eb; margin-bottom: 0.5rem;">
                        <i class="fas fa-list-ol"></i> Configuration Workflow
                    </h4>
                    <ol style="padding-left: 1.5rem; color: #4b5563; line-height: 2;">
                        ${docs.workflow.map(w => `
                            <li>
                                <strong style="color: #1e3a8a;">${w.action}</strong>
                                <div style="color: #6b7280; font-size: 0.875rem; margin-top: 0.25rem;">${w.description}</div>
                            </li>
                        `).join('')}
                    </ol>
                </div>
                ` : ''}

                ${docs.bestPractices ? `
                <div class="doc-section" style="margin-bottom: 1.5rem;">
                    <h4 style="color: #2563eb; margin-bottom: 0.5rem;">
                        <i class="fas fa-star"></i> Best Practices
                    </h4>
                    ${docs.bestPractices.map(bp => `
                        <div style="margin-bottom: 1rem; padding: 1rem; background: #ecfdf5; border-left: 4px solid #10b981; border-radius: 4px;">
                            <div style="font-weight: 600; color: #065f46; margin-bottom: 0.5rem;">
                                ${bp.practice}
                            </div>
                            <p style="color: #4b5563; margin-bottom: 0.5rem; font-size: 0.875rem;"><strong>Why:</strong> ${bp.reason}</p>
                            <p style="color: #6b7280; font-size: 0.875rem;"><strong>Example:</strong> ${bp.example}</p>
                        </div>
                    `).join('')}
                </div>
                ` : ''}

                ${docs.troubleshooting ? `
                <div class="doc-section" style="margin-bottom: 1.5rem;">
                    <h4 style="color: #2563eb; margin-bottom: 0.5rem;">
                        <i class="fas fa-wrench"></i> Troubleshooting
                    </h4>
                    ${docs.troubleshooting.map(t => `
                        <details style="margin-bottom: 0.75rem; border: 1px solid #fee2e2; border-radius: 6px; background: #fef2f2;">
                            <summary style="padding: 0.75rem; cursor: pointer; font-weight: 600; color: #991b1b;">
                                ${t.issue}
                            </summary>
                            <div style="padding: 1rem; background: white; border-top: 1px solid #fee2e2;">
                                <p style="color: #6b7280; font-size: 0.875rem; margin-bottom: 0.5rem;"><strong>Cause:</strong> ${t.cause}</p>
                                <p style="color: #059669; font-size: 0.875rem;"><strong>Fix:</strong> ${t.fix}</p>
                            </div>
                        </details>
                    `).join('')}
                </div>
                ` : ''}

                ${docs.securityNotes ? `
                <div class="doc-section" style="margin-bottom: 1.5rem;">
                    <h4 style="color: #2563eb; margin-bottom: 0.5rem;">
                        <i class="fas fa-shield-alt"></i> Security Notes
                    </h4>
                    ${docs.securityNotes.map(sn => `
                        <div style="margin-bottom: 1rem; padding: 1rem; background: #fff7ed; border-left: 4px solid #f97316; border-radius: 4px;">
                            <div style="font-weight: 600; color: #9a3412; margin-bottom: 0.5rem;">
                                ${sn.note}
                            </div>
                            <p style="color: #4b5563; margin-bottom: 0.5rem; font-size: 0.875rem;">${sn.detail}</p>
                            <p style="color: #059669; font-size: 0.875rem;"><strong>Recommendation:</strong> ${sn.recommendation}</p>
                        </div>
                    `).join('')}
                </div>
                ` : ''}

                ${docs.accessMethods ? `
                <div class="doc-section" style="margin-bottom: 1.5rem;">
                    <h4 style="color: #2563eb; margin-bottom: 0.5rem;">
                        <i class="fas fa-code-branch"></i> Access Methods
                    </h4>
                    ${docs.accessMethods.map(am => `
                        <div style="margin-bottom: 1rem; padding: 1rem; background: #f0f9ff; border-left: 4px solid #0284c7; border-radius: 4px;">
                            <div style="font-weight: 600; color: #075985; margin-bottom: 0.5rem;">
                                ${am.method}
                            </div>
                            <p style="color: #4b5563; margin-bottom: 0.5rem; font-size: 0.875rem;">${am.description}</p>
                            <p style="color: #6b7280; font-size: 0.875rem; margin-bottom: 0.5rem;"><strong>Format:</strong> <code style="background: white; padding: 0.2rem 0.5rem; border-radius: 3px;">${am.format}</code></p>
                            <details style="margin-top: 0.5rem;">
                                <summary style="cursor: pointer; color: #0284c7; font-size: 0.875rem; font-weight: 600;">View Examples</summary>
                                <ul style="margin-top: 0.5rem; padding-left: 1.5rem; font-size: 0.875rem;">
                                    ${am.examples.map(ex => `<li><code style="background: white; padding: 0.2rem 0.5rem; border-radius: 3px;">${ex}</code></li>`).join('')}
                                </ul>
                            </details>
                        </div>
                    `).join('')}
                </div>
                ` : ''}

                ${docs.stepOutputStructure ? `
                <div class="doc-section" style="margin-bottom: 1.5rem;">
                    <h4 style="color: #2563eb; margin-bottom: 0.5rem;">
                        <i class="fas fa-sitemap"></i> Step Output Structure Examples
                    </h4>
                    <p style="color: #6b7280; font-size: 0.875rem; margin-bottom: 1rem;">Click to expand and see the data structure for each step type.</p>
                    ${Object.keys(docs.stepOutputStructure).map(stepType => {
                        const structure = docs.stepOutputStructure[stepType];
                        return `
                        <details style="margin-bottom: 0.75rem; border: 1px solid #e5e7eb; border-radius: 6px; background: #f9fafb;">
                            <summary style="padding: 0.75rem; cursor: pointer; font-weight: 600; color: #1e3a8a;">
                                ${stepType.replace(/_/g, ' ').toUpperCase()}
                            </summary>
                            <div style="padding: 1rem; background: white; border-top: 1px solid #e5e7eb;">
                                <pre style="background: #f3f4f6; padding: 1rem; border-radius: 6px; overflow-x: auto; font-size: 0.8rem; line-height: 1.6;"><code>${JSON.stringify(structure, null, 2)}</code></pre>
                            </div>
                        </details>
                        `;
                    }).join('')}
                </div>
                ` : ''}

                ${docs.oobTemplates ? `
                <div class="doc-section" style="margin-bottom: 1.5rem;">
                    <h4 style="color: #2563eb; margin-bottom: 0.5rem;">
                        <i class="fas fa-table"></i> Built-in Templates
                    </h4>
                    <p style="color: #6b7280; font-size: 0.875rem; margin-bottom: 0.75rem;">Select a template in the Format tab — column definitions are applied automatically. No manual mapping needed.</p>
                    <table style="width: 100%; border-collapse: collapse; font-size: 0.875rem;">
                        <thead>
                            <tr style="background: #f3f4f6;">
                                <th style="padding: 0.6rem; text-align: left; border: 1px solid #e5e7eb;">Template Key</th>
                                <th style="padding: 0.6rem; text-align: left; border: 1px solid #e5e7eb;">Name</th>
                                <th style="padding: 0.6rem; text-align: left; border: 1px solid #e5e7eb;">Notes</th>
                            </tr>
                        </thead>
                        <tbody>
                            ${docs.oobTemplates.map(t => `
                                <tr>
                                    <td style="padding: 0.6rem; border: 1px solid #e5e7eb;"><code style="background: #e0e7ff; padding: 0.2rem 0.4rem; border-radius: 3px;">${t.key}</code></td>
                                    <td style="padding: 0.6rem; border: 1px solid #e5e7eb; font-weight: 500;">${t.name}</td>
                                    <td style="padding: 0.6rem; border: 1px solid #e5e7eb; color: #6b7280;">${t.note}</td>
                                </tr>
                            `).join('')}
                        </tbody>
                    </table>
                </div>
                ` : ''}

                ${docs.stepOutput ? `
                <div class="doc-section" style="margin-bottom: 1.5rem;">
                    <h4 style="color: #2563eb; margin-bottom: 0.5rem;">
                        <i class="fas fa-poll"></i> Step Output Variables
                    </h4>
                    <p style="color: #6b7280; font-size: 0.875rem; margin-bottom: 0.75rem;">${docs.stepOutput.description}</p>
                    <table style="width: 100%; border-collapse: collapse; font-size: 0.875rem;">
                        <thead>
                            <tr style="background: #f3f4f6;">
                                <th style="padding: 0.6rem; text-align: left; border: 1px solid #e5e7eb;">Variable</th>
                                <th style="padding: 0.6rem; text-align: left; border: 1px solid #e5e7eb;">Type</th>
                                <th style="padding: 0.6rem; text-align: left; border: 1px solid #e5e7eb;">Description</th>
                            </tr>
                        </thead>
                        <tbody>
                            ${docs.stepOutput.fields.map(f => `
                                <tr>
                                    <td style="padding: 0.6rem; border: 1px solid #e5e7eb;"><code style="background: #dcfce7; padding: 0.2rem 0.4rem; border-radius: 3px; color: #166534;">${f.name}</code></td>
                                    <td style="padding: 0.6rem; border: 1px solid #e5e7eb;"><code style="font-size: 0.8rem;">${f.type}</code></td>
                                    <td style="padding: 0.6rem; border: 1px solid #e5e7eb; color: #4b5563;">${f.description}</td>
                                </tr>
                            `).join('')}
                        </tbody>
                    </table>
                </div>
                ` : ''}

                ${docs.parameters ? `
                <div class="doc-section">
                    <h4 style="color: #2563eb; margin-bottom: 0.5rem;">
                        <i class="fas fa-cog"></i> Field Reference
                    </h4>
                    <table style="width: 100%; border-collapse: collapse;">
                        <thead>
                            <tr style="background: #f3f4f6;">
                                <th style="padding: 0.75rem; text-align: left; border: 1px solid #e5e7eb;">Field Key</th>
                                <th style="padding: 0.75rem; text-align: left; border: 1px solid #e5e7eb;">Type</th>
                                <th style="padding: 0.75rem; text-align: left; border: 1px solid #e5e7eb;">Required</th>
                                <th style="padding: 0.75rem; text-align: left; border: 1px solid #e5e7eb;">Description</th>
                            </tr>
                        </thead>
                        <tbody>
                            ${docs.parameters.map(p => {
                                const depth = (p.name.match(/\./g) || []).length;
                                const indent = depth > 0 ? `padding-left: ${depth * 16 + 12}px;` : 'padding: 0.75rem;';
                                const nameColor = depth === 0 ? '#059669' : depth === 1 ? '#0891b2' : '#7c3aed';
                                const prefix = depth > 0 ? '└ ' : '';
                                const rowBg = depth > 0 ? 'background: #fafafa;' : '';
                                return `
                                <tr style="${rowBg}">
                                    <td style="${indent} padding-top: 0.6rem; padding-bottom: 0.6rem; border: 1px solid #e5e7eb; font-family: monospace; color: ${nameColor}; white-space: nowrap;">${prefix}${p.name}</td>
                                    <td style="padding: 0.6rem 0.75rem; border: 1px solid #e5e7eb;"><code style="font-size: 0.85rem;">${p.type}</code></td>
                                    <td style="padding: 0.6rem 0.75rem; border: 1px solid #e5e7eb; text-align: center;">${p.required ? '✓' : '-'}</td>
                                    <td style="padding: 0.6rem 0.75rem; border: 1px solid #e5e7eb; line-height: 1.5;">${p.description}</td>
                                </tr>`;
                            }).join('')}
                        </tbody>
                    </table>
                </div>
                ` : ''}

                ${docs.assemblyRulesDoc ? `
                <div class="doc-section" style="margin-bottom: 1.5rem;">
                    <h4 style="color: #2563eb; margin-bottom: 0.5rem;">
                        <i class="fas fa-microscope"></i> Assembly Rules Reference
                    </h4>
                    <p style="color: #6b7280; font-size: 0.875rem; margin-bottom: 0.75rem;">
                        Each rule can be toggled individually in the <strong>Assembly</strong> tab.
                        All rules are on by default. Set a rule to off to skip that specific transform —
                        useful when your interface provides that field differently or you handle it in a downstream Script step.
                    </p>
                    <table style="width: 100%; border-collapse: collapse; font-size: 0.85rem;">
                        <thead>
                            <tr style="background: #f3f4f6;">
                                <th style="padding: 0.6rem 0.75rem; text-align: left; border: 1px solid #e5e7eb; white-space: nowrap;">Rule key</th>
                                <th style="padding: 0.6rem 0.75rem; text-align: left; border: 1px solid #e5e7eb; white-space: nowrap;">HL7 source</th>
                                <th style="padding: 0.6rem 0.75rem; text-align: left; border: 1px solid #e5e7eb; white-space: nowrap;">FHIR target</th>
                                <th style="padding: 0.6rem 0.75rem; text-align: left; border: 1px solid #e5e7eb;">What it does</th>
                            </tr>
                        </thead>
                        <tbody>
                            ${docs.assemblyRulesDoc.map((r, i) => {
                                const isObs = r.key.startsWith('obs_');
                                const bg = i % 2 === 0 ? '' : 'background:#fafafa;';
                                const groupBorder = !isObs && docs.assemblyRulesDoc[i-1]?.key.startsWith('obs_') ? 'border-top: 2px solid #e5e7eb;' : '';
                                return `<tr style="${bg}${groupBorder}">
                                    <td style="padding: 0.55rem 0.75rem; border: 1px solid #e5e7eb; font-family: monospace; font-size: 0.78rem; color: #7c3aed; white-space: nowrap;">${r.key}</td>
                                    <td style="padding: 0.55rem 0.75rem; border: 1px solid #e5e7eb; white-space: nowrap;"><code style="background:#dbeafe;padding:1px 5px;border-radius:3px;color:#1e3a8a;font-size:0.78rem;">${r.src}</code></td>
                                    <td style="padding: 0.55rem 0.75rem; border: 1px solid #e5e7eb; white-space: nowrap;"><code style="background:#fce7f3;padding:1px 5px;border-radius:3px;color:#831843;font-size:0.78rem;">${r.fhir}</code></td>
                                    <td style="padding: 0.55rem 0.75rem; border: 1px solid #e5e7eb; line-height: 1.5; color: #374151;">${r.desc}</td>
                                </tr>`;
                            }).join('')}
                        </tbody>
                    </table>
                </div>
                ` : ''}

                ${docs.actions ? `
                <div class="doc-section" style="margin-bottom: 1.5rem;">
                    <h4 style="color: #2563eb; margin-bottom: 0.5rem;">
                        <i class="fas fa-bolt"></i> Available Actions
                    </h4>
                    <div style="display: grid; gap: 0.75rem;">
                        ${docs.actions.map(a => `
                            <div style="padding: 1rem; background: #f8fafc; border: 1px solid #e2e8f0; border-radius: 6px;">
                                <div style="display: flex; align-items: center; gap: 0.5rem; margin-bottom: 0.5rem;">
                                    <code style="background: #3b82f6; color: white; padding: 0.25rem 0.5rem; border-radius: 4px; font-weight: 600;">${a.action}</code>
                                </div>
                                <p style="color: #475569; font-size: 0.875rem; margin-bottom: 0.5rem;">${a.description}</p>
                                <p style="color: #64748b; font-size: 0.8rem;"><strong>Use for:</strong> ${a.usedFor}</p>
                                ${a.parameters ? `<p style="color: #64748b; font-size: 0.8rem;"><strong>Parameters:</strong> ${a.parameters}</p>` : ''}
                            </div>
                        `).join('')}
                    </div>
                </div>
                ` : ''}

                ${docs.multiStepRouting ? `
                <div class="doc-section" style="margin-bottom: 1.5rem;">
                    <h4 style="color: #2563eb; margin-bottom: 0.5rem;">
                        <i class="fas fa-route"></i> ${docs.multiStepRouting.title}
                    </h4>
                    <p style="color: #4b5563; margin-bottom: 1rem;">${docs.multiStepRouting.description}</p>

                    <div style="background: #f0f9ff; border: 1px solid #bae6fd; border-radius: 6px; padding: 1rem; margin-bottom: 1rem;">
                        <h5 style="color: #0369a1; margin-bottom: 0.5rem;">How to Use</h5>
                        <ol style="padding-left: 1.5rem; color: #0369a1; font-size: 0.875rem; line-height: 1.8;">
                            ${docs.multiStepRouting.howToUse.map(step => `<li>${step}</li>`).join('')}
                        </ol>
                    </div>

                    ${docs.multiStepRouting.example ? `
                    <div style="background: #f8fafc; border: 1px solid #e2e8f0; border-radius: 6px; padding: 1rem;">
                        <h5 style="color: #334155; margin-bottom: 0.5rem;">Example: ${docs.multiStepRouting.example.scenario}</h5>
                        <pre style="background: #1e293b; color: #e2e8f0; padding: 0.75rem; border-radius: 4px; font-size: 0.8rem; overflow-x: auto;"><code>${JSON.stringify(docs.multiStepRouting.example.config, null, 2)}</code></pre>
                        <p style="color: #64748b; font-size: 0.8rem; margin-top: 0.5rem;"><strong>Execution:</strong> ${docs.multiStepRouting.example.execution}</p>
                    </div>
                    ` : ''}
                </div>
                ` : ''}

                ${docs.comparisonWithIfThenElse ? `
                <div class="doc-section" style="margin-bottom: 1.5rem;">
                    <h4 style="color: #2563eb; margin-bottom: 0.5rem;">
                        <i class="fas fa-balance-scale"></i> ${docs.comparisonWithIfThenElse.title}
                    </h4>
                    ${docs.comparisonWithIfThenElse.description ? `<p style="color: #4b5563; margin-bottom: 1rem;">${docs.comparisonWithIfThenElse.description}</p>` : ''}
                    <table style="width: 100%; border-collapse: collapse; font-size: 0.875rem;">
                        <thead>
                            <tr style="background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);">
                                <th style="padding: 0.75rem; text-align: left; border: 1px solid #e5e7eb; color: white;">Feature</th>
                                <th style="padding: 0.75rem; text-align: left; border: 1px solid #e5e7eb; color: white;">Switch/Case</th>
                                <th style="padding: 0.75rem; text-align: left; border: 1px solid #e5e7eb; color: white;">If-Then-Else</th>
                            </tr>
                        </thead>
                        <tbody>
                            ${docs.comparisonWithIfThenElse.comparison.map((row, i) => `
                                <tr style="background: ${i % 2 === 0 ? '#f8fafc' : 'white'};">
                                    <td style="padding: 0.75rem; border: 1px solid #e5e7eb; font-weight: 600; color: #334155;">${row.feature}</td>
                                    <td style="padding: 0.75rem; border: 1px solid #e5e7eb; color: #059669;">${row.switchCase}</td>
                                    <td style="padding: 0.75rem; border: 1px solid #e5e7eb; color: #7c3aed;">${row.ifThenElse}</td>
                                </tr>
                            `).join('')}
                        </tbody>
                    </table>
                    ${docs.comparisonWithIfThenElse.recommendation ? `
                    <div style="margin-top: 1rem; padding: 0.75rem; background: #fef3c7; border-left: 4px solid #f59e0b; border-radius: 4px;">
                        <p style="color: #92400e; font-size: 0.875rem; margin: 0;"><strong>Recommendation:</strong> ${docs.comparisonWithIfThenElse.recommendation}</p>
                    </div>
                    ` : ''}
                </div>
                ` : ''}
            </div>
        `;

        // Wire connector type documentation dropdown
        if (docs.connectorTypeCards) {
            const sel = container.querySelector('#connectorTypeDocSelect');
            const detail = container.querySelector('#connectorTypeDocDetail');
            if (sel && detail) {
                sel.addEventListener('change', () => {
                    const ct = docs.connectorTypeCards.find(c => c.typeName === sel.value);
                    if (!ct) { detail.style.display = 'none'; return; }
                    const scriptRows = ct.example && JSON.stringify(ct.example, null, 2).replace(/\\n/g, '\n');
                    detail.style.display = '';
                    detail.innerHTML = `
                        <div style="border:1px solid #e5e7eb;border-radius:6px;background:#f9fafb;overflow:hidden;">
                            <div style="padding:0.7rem 1rem;background:#1e3a8a;color:white;display:flex;align-items:center;gap:0.6rem;">
                                <span style="font-size:1.1rem;">${ct.icon}</span>
                                <strong>${ct.displayName}</strong>
                                <code style="background:rgba(255,255,255,0.2);padding:0.1rem 0.5rem;border-radius:3px;font-size:0.75rem;">${ct.typeName}</code>
                                <span style="margin-left:auto;font-size:0.75rem;background:${ct.mode==='push'?'#dcfce7':'#fef3c7'};color:${ct.mode==='push'?'#166534':'#92400e'};padding:0.1rem 0.5rem;border-radius:10px;">${ct.mode==='push'?'⚡ push (long-lived listener)':'🔄 pull (cron / scheduled)'}</span>
                            </div>
                            <div style="padding:0.75rem 1rem;">
                                <p style="color:#4b5563;font-size:0.875rem;margin-bottom:0.6rem;">${ct.description}</p>
                                ${ct.notes ? `<p style="color:#92400e;background:#fffbeb;border:1px solid #fde68a;border-radius:4px;padding:0.4rem 0.7rem;font-size:0.8rem;margin-bottom:0.6rem;"><strong>⚠ Note:</strong> ${ct.notes}</p>` : ''}
                                <div style="margin-bottom:0.75rem;">
                                    <strong style="font-size:0.8rem;color:#374151;">Required fields: </strong>
                                    ${ct.required.map(f => `<code style="background:#fee2e2;color:#991b1b;padding:0.1rem 0.4rem;border-radius:3px;font-size:0.75rem;margin-right:3px;">${f}</code>`).join('')}
                                </div>
                                ${ct.keyFields && ct.keyFields.length ? `
                                <table style="width:100%;border-collapse:collapse;font-size:0.8rem;margin-bottom:0.75rem;">
                                    <thead><tr style="background:#f3f4f6;">
                                        <th style="padding:0.4rem 0.6rem;text-align:left;border:1px solid #e5e7eb;">Field</th>
                                        <th style="padding:0.4rem 0.6rem;text-align:left;border:1px solid #e5e7eb;">Type</th>
                                        <th style="padding:0.4rem 0.6rem;text-align:left;border:1px solid #e5e7eb;">Default</th>
                                        <th style="padding:0.4rem 0.6rem;text-align:left;border:1px solid #e5e7eb;">Notes</th>
                                    </tr></thead>
                                    <tbody>${ct.keyFields.map(f => `<tr>
                                        <td style="padding:0.4rem 0.6rem;border:1px solid #e5e7eb;font-family:monospace;color:#0891b2;">${f.name}${f.required?' <span style="color:#ef4444">*</span>':''}</td>
                                        <td style="padding:0.4rem 0.6rem;border:1px solid #e5e7eb;"><code>${f.type}</code></td>
                                        <td style="padding:0.4rem 0.6rem;border:1px solid #e5e7eb;color:#6b7280;">${f.default || '—'}</td>
                                        <td style="padding:0.4rem 0.6rem;border:1px solid #e5e7eb;color:#4b5563;">${f.notes || ''}</td>
                                    </tr>`).join('')}</tbody>
                                </table>` : ''}
                                <details style="margin-top:0.25rem;">
                                    <summary style="cursor:pointer;color:#2563eb;font-size:0.8rem;font-weight:600;">📋 View example config</summary>
                                    <pre style="background:#1e1e2e;color:#cdd6f4;padding:0.75rem;border-radius:4px;margin-top:0.5rem;font-size:0.78rem;overflow-x:auto;line-height:1.5;"><code>${scriptRows}</code></pre>
                                </details>
                            </div>
                        </div>
                    `;
                });
            }
        }

        return container;
    }

    /**
     * Create properties form (legacy method - now used by JSON tab)
     */
    createPropertiesForm(step) {
        const form = document.createElement('div');
        form.className = 'properties-form';

        form.innerHTML = `
            <h3 style="margin-bottom: 1rem; display: flex; align-items: center; gap: 0.5rem;">
                <i class="${step.icon}"></i>
                <span>Step Configuration</span>
            </h3>

            ${this.createBasicPropertiesSection(step)}
            ${this.createExecutionPropertiesSection(step)}
            ${this.createConfigSection(step)}
            ${step.stepType === 'custom' || step.scriptContent ? this.createScriptSection(step) : ''}

            <div class="form-actions" style="margin-top: 1.5rem; display: flex; gap: 0.5rem;">
                <button class="btn btn-primary" id="saveStepBtn">
                    <i class="fas fa-save"></i> Save
                </button>
                <button class="btn btn-secondary" id="cancelStepBtn">
                    <i class="fas fa-times"></i> Cancel
                </button>
            </div>
        `;

        // Attach event listeners
        this.attachFormEvents(form, step);

        return form;
    }

    /**
     * Create basic properties section
     */
    createBasicPropertiesSection(step) {
        return `
            <div class="form-section">
                <h4>Basic Properties</h4>

                <div class="form-group">
                    <label>Step Name *</label>
                    <input type="text" id="stepName" value="${step.stepName}" required>
                </div>

                <div class="form-group">
                    <label>Description</label>
                    <textarea id="stepDescription" rows="3">${step.description || ''}</textarea>
                </div>

                <!-- Icon is automatically assigned based on step type -->
            </div>
        `;
    }

    /**
     * Create execution properties section
     */
    createExecutionPropertiesSection(step) {
        return `
            <div class="form-section">
                <h4>Execution Settings</h4>

                <div class="form-group-inline">
                    <div class="form-group">
                        <label>Sequence <span style="font-weight: normal; color: #6b7280; font-size: 11px;">(auto from connections)</span></label>
                        <input type="number" id="stepSequence" value="${step.sequence}" min="1" readonly
                               style="background: #f3f4f6; cursor: not-allowed;"
                               title="Sequence is auto-calculated from flowchart connections on save">
                    </div>

                    <div class="form-group">
                        <label>Timeout (ms)</label>
                        <input type="number" id="stepTimeout" value="${step.timeoutMs}" min="100">
                    </div>
                </div>

                <div class="form-group">
                    <label>On Error Strategy</label>
                    <select id="stepErrorStrategy">
                        <option value="fail" ${step.onErrorStrategy === 'fail' ? 'selected' : ''}>Fail (Stop pipeline)</option>
                        <option value="skip" ${step.onErrorStrategy === 'skip' ? 'selected' : ''}>Skip (Continue)</option>
                        <option value="default" ${step.onErrorStrategy === 'default' ? 'selected' : ''}>Use Default Value</option>
                    </select>
                </div>

                <div class="form-group">
                    <label style="display: flex; align-items: center; gap: 0.5rem;">
                        <input type="checkbox" id="stepRequired" ${step.required ? 'checked' : ''}>
                        <span>Required Step</span>
                    </label>
                </div>

                <div class="form-group">
                    <label style="display: flex; align-items: center; gap: 0.5rem;">
                        <input type="checkbox" id="stepEnabled" ${step.enabled !== false ? 'checked' : ''}>
                        <span>Enabled</span>
                    </label>
                </div>
            </div>
        `;
    }

    /**
     * Create universal Error Handling section for all steps.
     * Provides per-step try-catch with built-in catch actions.
     */
    createErrorHandlingSection(step) {
        const eh = step.config?.errorHandling || {};
        const isEnabled = eh.enabled || false;
        const onError = (eh.onError === 'catch' ? 'suppress' : eh.onError) || 'suppress';
        const defaultField = eh.defaultField || '';
        const defaultValue = eh.defaultValue || '';

        // Retry config (per-step retry)
        const retry = step.config?.retry || {};
        const retryEnabled = retry.enabled || false;
        const maxRetries = retry.maxRetries || 3;
        const delayMs = retry.delayMs || 1000;
        const backoffMultiplier = retry.backoffMultiplier || 2;

        // Inheritance detection: check if pipeline has defaults
        const pipelineConfig = window.pipelineBuilder?.pipeline?.pipelineConfig || {};
        const hasRetryDefault = pipelineConfig.defaultRetry?.enabled;
        const hasEHDefault = pipelineConfig.defaultErrorHandling?.enabled;
        const hasStepRetry = step.config?.retry !== undefined;
        const hasStepEH = step.config?.errorHandling !== undefined;

        // Build inheritance badges
        let inheritanceBadge = '';
        if ((hasRetryDefault || hasEHDefault) && !hasStepRetry && !hasStepEH) {
            inheritanceBadge = '<span style="font-size:10px; font-weight:normal; color:#3b82f6; background:rgba(59,130,246,0.1); padding:1px 6px; border-radius:4px;">Inherited</span>';
        } else if ((hasRetryDefault || hasEHDefault) && (hasStepRetry || hasStepEH)) {
            inheritanceBadge = '<span style="font-size:10px; font-weight:normal; color:#8b5cf6; background:rgba(139,92,246,0.1); padding:1px 6px; border-radius:4px;">Override</span>';
        }

        return `
            <div class="form-section error-handling-section">
                <h4 style="display:flex; align-items:center; gap:8px; cursor:pointer;" onclick="
                    const body = this.nextElementSibling;
                    const arrow = this.querySelector('.eh-arrow');
                    if (body.style.display === 'none') {
                        body.style.display = 'block';
                        arrow.style.transform = 'rotate(90deg)';
                    } else {
                        body.style.display = 'none';
                        arrow.style.transform = 'rotate(0deg)';
                    }
                ">
                    <span class="eh-arrow" style="transition:0.2s; transform:rotate(${isEnabled || retryEnabled ? '90deg' : '0deg'}); font-size:12px;">&#9654;</span>
                    Error Handling & Retry
                    ${isEnabled ? '<span style="font-size:10px; font-weight:normal; color:#ef4444; background:rgba(239,68,68,0.1); padding:1px 6px; border-radius:4px;">ERROR ON</span>' : ''}
                    ${retryEnabled ? '<span style="font-size:10px; font-weight:normal; color:#8b5cf6; background:rgba(139,92,246,0.1); padding:1px 6px; border-radius:4px;">RETRY ON</span>' : ''}
                    ${inheritanceBadge}
                </h4>
                <div style="display:${isEnabled || retryEnabled ? 'block' : 'none'};">

                    <!-- Retry Section -->
                    <div style="margin-bottom:12px; padding:10px; border:1px solid rgba(139,92,246,0.2); border-radius:6px; background:rgba(139,92,246,0.03);">
                        <div style="display:flex; align-items:center; gap:8px; margin-bottom:8px;">
                            <label style="position:relative; display:inline-block; width:36px; height:20px; cursor:pointer;">
                                <input type="checkbox" id="retryEnabled" ${retryEnabled ? 'checked' : ''}
                                    style="opacity:0; width:0; height:0;">
                                <span style="position:absolute; top:0; left:0; right:0; bottom:0;
                                    background:${retryEnabled ? '#8b5cf6' : '#cbd5e1'}; border-radius:10px; transition:0.2s;"></span>
                                <span style="position:absolute; top:2px; left:${retryEnabled ? '18px' : '2px'};
                                    width:16px; height:16px; background:white; border-radius:50%; transition:0.2s;"></span>
                            </label>
                            <span style="font-size:12px; font-weight:500;"><i class="fas fa-sync" style="color:#8b5cf6; margin-right:4px;"></i>Retry on failure</span>
                        </div>
                        <div id="retryConfigArea" style="${!retryEnabled ? 'opacity:0.4; pointer-events:none;' : ''}">
                            <div style="display:flex; gap:8px;">
                                <div style="flex:1;">
                                    <label style="font-size:10px; color:var(--text-secondary);">Max Retries</label>
                                    <input type="number" id="retryMaxRetries" class="form-control"
                                        value="${maxRetries}" min="1" max="10"
                                        style="font-size:11px; padding:4px 8px; margin-top:2px;">
                                </div>
                                <div style="flex:1;">
                                    <label style="font-size:10px; color:var(--text-secondary);">Delay (ms)</label>
                                    <input type="number" id="retryDelayMs" class="form-control"
                                        value="${delayMs}" min="0" step="100"
                                        style="font-size:11px; padding:4px 8px; margin-top:2px;">
                                </div>
                                <div style="flex:1;">
                                    <label style="font-size:10px; color:var(--text-secondary);">Backoff x</label>
                                    <input type="number" id="retryBackoffMultiplier" class="form-control"
                                        value="${backoffMultiplier}" min="1" max="5" step="0.5"
                                        style="font-size:11px; padding:4px 8px; margin-top:2px;">
                                </div>
                            </div>
                            <p style="font-size:10px; color:var(--text-secondary); margin:6px 0 0;">
                                Retries with exponential backoff: ${delayMs}ms, ${delayMs * backoffMultiplier}ms, ${delayMs * backoffMultiplier * backoffMultiplier}ms...
                            </p>
                        </div>
                    </div>

                    <!-- Error Handling Section -->
                    <div style="padding:10px; border:1px solid rgba(239,68,68,0.2); border-radius:6px; background:rgba(239,68,68,0.03);">
                        <div style="display:flex; align-items:center; gap:8px; margin-bottom:8px;">
                            <label style="position:relative; display:inline-block; width:36px; height:20px; cursor:pointer;">
                                <input type="checkbox" id="ehEnabled" ${isEnabled ? 'checked' : ''}
                                    style="opacity:0; width:0; height:0;">
                                <span style="position:absolute; top:0; left:0; right:0; bottom:0;
                                    background:${isEnabled ? '#ef4444' : '#cbd5e1'}; border-radius:10px; transition:0.2s;"></span>
                                <span style="position:absolute; top:2px; left:${isEnabled ? '18px' : '2px'};
                                    width:16px; height:16px; background:white; border-radius:50%; transition:0.2s;"></span>
                            </label>
                            <span style="font-size:12px; font-weight:500;"><i class="fas fa-shield-alt" style="color:#ef4444; margin-right:4px;"></i>Error handling (after retries exhausted)</span>
                        </div>

                        <div id="ehConfigArea" style="${!isEnabled ? 'opacity:0.4; pointer-events:none;' : ''}">
                            <div class="form-group" style="margin-bottom:10px;">
                                <label style="font-size:12px; font-weight:600;">On Error</label>
                                <select id="ehOnError" class="form-control" style="font-size:12px; margin-top:3px;">
                                    <option value="suppress" ${onError === 'suppress' ? 'selected' : ''}>Suppress - Log error, continue pipeline</option>
                                    <option value="rethrow" ${onError === 'rethrow' ? 'selected' : ''}>Rethrow - Stop pipeline on error</option>
                                </select>
                            </div>

                            <div style="border-top:1px solid var(--border-color, #e2e8f0); padding-top:10px; margin-top:10px;">
                                <label style="font-size:11px; font-weight:700; text-transform:uppercase; letter-spacing:0.5px; color:#10b981;">
                                    <i class="fas fa-edit" style="margin-right:4px;"></i> Default Value (optional)
                                </label>
                                <p style="font-size:10px; color:var(--text-secondary); margin:2px 0 8px;">Set a fallback value for a field when this step fails</p>
                                <div style="display:flex; gap:8px;">
                                    <div style="flex:1;">
                                        <label style="font-size:10px; color:var(--text-secondary);">Field name</label>
                                        <input type="text" id="ehDefaultField" class="form-control"
                                            value="${defaultField}" placeholder="e.g. patient_status"
                                            style="font-size:11px; padding:4px 8px; margin-top:2px;">
                                    </div>
                                    <div style="flex:1;">
                                        <label style="font-size:10px; color:var(--text-secondary);">Fallback value</label>
                                        <input type="text" id="ehDefaultValue" class="form-control"
                                            value="${defaultValue}" placeholder="e.g. unknown"
                                            style="font-size:11px; padding:4px 8px; margin-top:2px;">
                                    </div>
                                </div>
                            </div>
                        </div>
                    </div>

                </div>
            </div>
        `;
    }

    /**
     * Create configuration section
     */
    createConfigSection(step) {
        // Use VisualStep utility for step type detection (handles old and new type names)
        const isHL7FHIR = VisualStep.isHL7FHIRTransform(step);
        console.log('🔍 createConfigSection called with:', {
            stepType: step.stepType,
            templateId: step.templateId,
            stepName: step.name,
            willUseMappingUI: isHL7FHIR
        });

        // Special handling for HL7→FHIR mapping steps ONLY (not generic transformation)
        if (isHL7FHIR) {
            return this.createMappingConfigSection(step);
        }

        // Default JSON editor for other step types
        const configJSON = JSON.stringify(step.config, null, 2);

        return `
            <div class="form-section">
                <h4>Step Configuration</h4>

                <div class="form-group">
                    <label>Configuration (JSON)</label>
                    <textarea id="stepConfig" rows="8" style="font-family: 'Courier New', monospace;">${configJSON}</textarea>
                    <small style="color: #64748b;">Enter configuration as JSON object</small>
                </div>
            </div>
        `;
    }

    /**
     * Create HL7→FHIR mapping configuration section (enhanced UI)
     * Uses reference-based template system: step stores template reference,
     * UI fetches and displays template mappings + any custom overrides
     */
    createMappingConfigSection(step) {
        console.log('🗺️ ========== HL7-FHIR MAPPING DEBUG ==========');
        console.log('🗺️ step.config:', step.config);
        console.log('🗺️ use_standard_template:', step.config?.use_standard_template);
        console.log('🗺️ =============================================');

        // interface_message_mappings is the single source of truth.
        // If the step carries interface_id we always fetch the resolved mapping live
        // from GET /api/fhir/interfaces/:id/resolved-mappings so the UI shows what
        // the runtime chain actually uses (including Z-segments, AI additions, deltas).
        const interfaceId = step.config?.interface_id || this.builder.pipeline?.interfaceId;
        const messageType = step.config?.message_type || this.builder.pipeline?.messageType || 'ADT^A01';
        const mappingMode = step.config?.mapping_mode || 'oob';

        let mappings = [];
        let mappingSource = 'none';

        // Legacy embedded_mappings / config.mappings — only used if no interface_id present
        if (!interfaceId) {
            const embeddedMappings = step.config?.embedded_mappings;
            if (embeddedMappings) {
                if (Array.isArray(embeddedMappings)) mappings = embeddedMappings;
                else if (Array.isArray(embeddedMappings.atomicMappings)) mappings = embeddedMappings.atomicMappings;
                mappingSource = 'embedded';
            } else if (Array.isArray(step.config?.mappings) && step.config.mappings.length > 0) {
                mappings = step.config.mappings;
                mappingSource = 'config';
            }
        } else {
            // Has interface reference — resolved live from backend
            mappingSource = 'interface_ref';
        }

        console.log('🗺️ mappingSource:', mappingSource, '| interfaceId:', interfaceId, '| mode:', mappingMode);

        const mappingCount = mappings.length;
        const configJSON = JSON.stringify(step.config, null, 2);

        // Mapping mode pill — shown in the section header so users always know which
        // path the runtime will take for this interface.
        const modeMeta = {
            oob:    { label: '🔗 Tracking OOB template',  bg: '#e0f2fe', color: '#0369a1', tip: 'Mappings come from the standard OOB template. Updates to the template are picked up automatically at runtime.' },
            delta:  { label: '⚡ OOB + custom additions', bg: '#fef9c3', color: '#854d0e', tip: 'Base OOB template plus your additions. OOB updates still flow through; your additions are preserved.' },
            custom: { label: '✏️ Fully custom',           bg: '#fce7f3', color: '#9d174d', tip: 'Fully custom mapping — no automatic OOB updates. You own the full configuration.' },
        };
        const mode    = interfaceId ? (mappingMode || 'oob') : 'oob';
        const modeObj = modeMeta[mode] || modeMeta.oob;

        return `
            <div class="form-section">
                <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:8px;">
                    <h4 style="margin:0;">HL7→FHIR Mapping Configuration</h4>
                    <div style="display:flex;align-items:center;gap:6px;">
                        <span title="Message type this step is mapping. Comes from the interface configuration."
                              style="font-size:11px;padding:3px 10px;border-radius:12px;background:#f0fdf4;color:#166534;font-weight:600;cursor:help;border:1px solid #bbf7d0;font-family:monospace;">
                            ${messageType || 'unknown'}
                        </span>
                        ${interfaceId ? `
                        <span title="${modeObj.tip}"
                              style="font-size:11px;padding:3px 10px;border-radius:12px;background:${modeObj.bg};color:${modeObj.color};font-weight:600;cursor:help;border:1px solid ${modeObj.color}33;">
                            ${modeObj.label}
                        </span>` : ''}
                    </div>
                </div>

                <!-- Tab Navigation -->
                <div class="config-tabs" style="display: flex; gap: 0.5rem; margin-bottom: 1rem; border-bottom: 2px solid #e5e7eb;">
                    <button class="config-tab active" data-tab="visual" style="
                        padding: 0.5rem 1rem;
                        border: none;
                        background: none;
                        cursor: pointer;
                        border-bottom: 3px solid #1e3a8a;
                        margin-bottom: -2px;
                        font-weight: 600;
                        color: #1e3a8a;
                    ">
                        <i class="fas fa-table"></i> Visual Mapping
                    </button>
                    <button class="config-tab" data-tab="resources" style="
                        padding: 0.5rem 1rem;
                        border: none;
                        background: none;
                        cursor: pointer;
                        color: #64748b;
                        font-weight: 500;
                    ">
                        <i class="fas fa-sitemap"></i> Resources
                    </button>
                    <button class="config-tab" data-tab="json" style="
                        padding: 0.5rem 1rem;
                        border: none;
                        background: none;
                        cursor: pointer;
                        color: #64748b;
                        font-weight: 500;
                    ">
                        <i class="fas fa-code"></i> JSON Editor
                    </button>
                    <button class="config-tab" data-tab="upload" style="
                        padding: 0.5rem 1rem;
                        border: none;
                        background: none;
                        cursor: pointer;
                        color: #64748b;
                        font-weight: 500;
                    ">
                        <i class="fas fa-upload"></i> Upload
                    </button>
                    <button class="config-tab" data-tab="assembly" style="
                        padding: 0.5rem 1rem;
                        border: none;
                        background: none;
                        cursor: pointer;
                        color: #64748b;
                        font-weight: 500;
                    ">
                        <i class="fas fa-microscope"></i> Assembly
                    </button>
                </div>

                <!-- Tab 1: Visual Mapping -->
                <div class="config-tab-content" data-tab-content="visual">
                    <!-- Info box about reference variables -->
                    <div style="margin-bottom: 1rem; padding: 1rem; background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); border-radius: 8px; color: white;">
                        <div style="display: flex; align-items: start; gap: 0.75rem;">
                            <div style="font-size: 1.5rem;">💡</div>
                            <div style="flex: 1;">
                                <div style="font-weight: 600; margin-bottom: 0.5rem; font-size: 0.95rem;">Using Enriched Data in Mappings</div>
                                <div style="font-size: 0.85rem; line-height: 1.5; opacity: 0.95;">
                                    You can reference data from previous enrichment steps in your field mappings!
                                    <br><br>
                                    <strong>Examples:</strong><br>
                                    • Use database results: <code style="background: rgba(255,255,255,0.2); padding: 2px 6px; border-radius: 3px; font-size: 0.8rem;">["database_enrichment"].enriched_data.fieldName</code><br>
                                    • Use script calculations: <code style="background: rgba(255,255,255,0.2); padding: 2px 6px; border-radius: 3px; font-size: 0.8rem;">["Script_Enrichment"].enriched_data.riskScore</code><br>
                                    • Use API data: <code style="background: rgba(255,255,255,0.2); padding: 2px 6px; border-radius: 3px; font-size: 0.8rem;">["API_Enrichment"].enriched_data.externalId</code>
                                    <br><br>
                                    <strong>👉 Click the "Variables" tab above to see all available variables from previous steps with copy-paste ready XPaths!</strong>
                                </div>
                            </div>
                        </div>
                    </div>

                    <div id="mappingStatusBar" style="margin-bottom: 1rem; padding: 0.75rem; background: #f8fafc; border-radius: 6px; display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 0.5rem;">
                        <span id="mappingStatusText" style="color: #475569; font-size: 0.875rem;">
                            ${mappingSource === 'interface_ref'
                                ? '<i class="fas fa-spinner fa-spin"></i> Loading mappings…'
                                : `<strong>${mappingCount}</strong> mappings configured`
                            }
                            ${mappingSource === 'interface_ref' ? (() => {
                                const modeLabels = { oob: '🔗 Tracking OOB template', delta: '⚡ OOB + custom additions', custom: '✏️ Fully custom' };
                                return `<span style="margin-left:6px;font-size:11px;padding:2px 7px;border-radius:10px;background:#e0f2fe;color:#0369a1;font-weight:600;">${modeLabels[mappingMode] || mappingMode}</span>`;
                            })() : ''}
                            ${mappingSource === 'embedded' ? '<span style="color: #059669; font-weight: 500;">(from wizard)</span>' : ''}
                            ${mappingSource === 'config' && mappingCount > 0 ? '<span style="color: #3b82f6; font-weight: 500;">(custom)</span>' : ''}
                            ${mappingSource === 'none' ? '<span style="color: #f59e0b; font-weight: 500;">(using standard template at runtime)</span>' : ''}
                        </span>
                        <div style="display: flex; gap: 0.5rem; align-items: center;">
                            <button id="loadStandardTemplateBtn" class="btn btn-secondary" style="font-size: 0.8rem; padding: 0.4rem 0.8rem;"
                                title="${mappingSource === 'interface_ref' ? 'Refresh resolved mappings from the interface configuration' : 'Load mappings from the standard template for this message type'}">
                                <i class="fas fa-${mappingSource === 'interface_ref' ? 'sync-alt' : 'download'}"></i>
                                ${mappingSource === 'interface_ref' ? 'Refresh Mappings' : 'Load Standard Template'}
                            </button>
                            <input type="text" id="mappingSearchInput" placeholder="Search mappings..." style="
                                padding: 0.375rem 0.75rem;
                                border: 1px solid #cbd5e1;
                                border-radius: 4px;
                                font-size: 0.875rem;
                                width: 200px;
                            ">
                        </div>
                    </div>

                    <div id="mappingTableContainer" style="
                        max-height: 400px;
                        overflow-y: auto;
                        border: 1px solid #e5e7eb;
                        border-radius: 6px;
                    " data-auto-load-template="${mappingSource === 'interface_ref' || mappingSource === 'template_reference' || mappingSource === 'none' ? 'true' : 'false'}"
                       data-interface-id="${interfaceId || ''}"
                       data-message-type="${messageType || ''}"
                       data-mapping-mode="${mappingMode || 'oob'}">
                        ${mappingSource === 'interface_ref' || mappingSource === 'template_reference' ?
                            '<div style="padding: 2rem; text-align: center; color: #64748b;"><i class="fas fa-spinner fa-spin fa-2x"></i><p style="margin-top: 1rem;">Loading mappings…</p></div>' :
                            this.renderMappingTable(mappings)
                        }
                    </div>

                    <div style="margin-top: 0.75rem; display: flex; gap: 0.5rem;">
                        <button id="addMappingBtn" class="btn btn-secondary" style="font-size: 0.875rem; padding: 0.5rem 1rem;">
                            <i class="fas fa-plus"></i> Add Mapping
                        </button>
                    </div>
                </div>

                <!-- Tab 2: Resources -->
                <div class="config-tab-content" data-tab-content="resources" style="display: none;">
                    <div id="resourceSelectorContainer" style="padding: 0.5rem 0;">
                        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem;">
                            <p style="color: #475569; font-size: 0.875rem; margin: 0;">
                                Choose which FHIR resources to include in the output. Deselect resources you don't need (e.g., MessageHeader).
                            </p>
                            <button id="loadResourcePreviewBtn" class="btn btn-secondary" style="font-size: 0.8rem; padding: 0.4rem 0.8rem; white-space: nowrap; margin-left: 1rem;">
                                <i class="fas fa-sync-alt"></i> Load
                            </button>
                        </div>
                        <div id="resourceCardsArea">
                            <div style="padding: 2rem; text-align: center; color: #94a3b8;">
                                <i class="fas fa-sitemap" style="font-size: 2rem; margin-bottom: 0.75rem; opacity: 0.4;"></i>
                                <p style="margin: 0; font-size: 0.875rem;">Click <strong>Load</strong> to see available FHIR resources for this message type</p>
                            </div>
                        </div>
                    </div>
                </div>

                <!-- Tab 3: JSON Editor -->
                <div class="config-tab-content" data-tab-content="json" style="display: none;">
                    <div class="form-group">
                        <label>Configuration (JSON)</label>
                        <textarea id="stepConfig" rows="15" style="font-family: 'Courier New', monospace; font-size: 0.875rem;">${configJSON}</textarea>
                        <small style="color: #64748b;">
                            Edit configuration as JSON. Structure: { mappings: [...], source: "wizard", ... }
                        </small>
                    </div>
                    <div style="display: flex; gap: 0.5rem; margin-top: 0.5rem;">
                        <button id="validateJsonBtn" class="btn btn-secondary" style="font-size: 0.875rem;">
                            <i class="fas fa-check-circle"></i> Validate JSON
                        </button>
                        <button id="formatJsonBtn" class="btn btn-secondary" style="font-size: 0.875rem;">
                            <i class="fas fa-magic"></i> Format
                        </button>
                    </div>
                </div>

                <!-- Tab 3: Upload -->
                <div class="config-tab-content" data-tab-content="upload" style="display: none;">
                    <div class="form-group">
                        <label>Upload Mapping Configuration</label>
                        <div style="
                            border: 2px dashed #cbd5e1;
                            border-radius: 8px;
                            padding: 2rem;
                            text-align: center;
                            background: #f8fafc;
                            cursor: pointer;
                        " id="uploadDropZone">
                            <i class="fas fa-cloud-upload-alt" style="font-size: 3rem; color: #94a3b8; margin-bottom: 1rem;"></i>
                            <p style="color: #64748b; margin-bottom: 0.5rem;">Drag and drop JSON file here</p>
                            <p style="color: #94a3b8; font-size: 0.875rem;">or</p>
                            <input type="file" id="uploadMappingFile" accept=".json" style="display: none;">
                            <button class="btn btn-primary" style="margin-top: 0.5rem;" onclick="document.getElementById('uploadMappingFile').click(); event.preventDefault();">
                                <i class="fas fa-folder-open"></i> Browse Files
                            </button>
                        </div>
                        <small style="color: #64748b; margin-top: 0.5rem; display: block;">
                            Supports JSON files with mapping configuration
                        </small>
                    </div>

                    <div class="form-group" style="margin-top: 1.5rem;">
                        <label>Export Current Mappings</label>
                        <button id="exportMappingsBtn" class="btn btn-secondary" style="width: 100%;">
                            <i class="fas fa-download"></i> Download as JSON
                        </button>
                    </div>
                </div>

                <!-- Tab 5: Assembly -->
                <div class="config-tab-content" data-tab-content="assembly" style="display: none;">
                    ${this._renderAssemblyTab(step)}
                </div>
            </div>
        `;
    }

    /**
     * Render Assembly tab — shows every OOB structural transform as a toggleable rule row.
     * Each rule key maps to a step.config.assemblyRules[key] boolean (default true).
     */
    _renderAssemblyTab(step) {
        const rules = step.config?.assemblyRules || {};
        const isEnabled = (key) => rules[key] !== false; // default ON

        // Master toggle: if assembleObservations is explicitly false, whole group is off
        const masterOn = step.config?.assembleObservations !== false;

        const OBS_RULES = [
            { key: 'obs_value_dispatch',  src: 'OBX.2 + OBX.5', fhir: 'value[x]',
              label: 'Value type dispatch',
              desc:  'Reads OBX.2 (data type) to decide which FHIR value field to populate from OBX.5.',
              example: 'OBX.2=NM, OBX.5=4.2        → "valueQuantity": {"value": 4.2}\nOBX.2=CE, OBX.5=N^Normal   → "valueCodeableConcept": {"coding": [{"code": "N"}]}\nOBX.2=TX, OBX.5=Report text → "valueString": "Report text"\nOBX.2=TS, OBX.5=20231015   → "valueDateTime": "2023-10-15"' },

            { key: 'obs_value_unit',      src: 'OBX.6',          fhir: 'valueQuantity.unit / system',
              label: 'Unit & coding system',
              desc:  'Attaches the unit to valueQuantity. Recognises UCUM as the standard coding system.',
              example: 'OBX.6=mmol/L^^UCUM → unit: "mmol/L", system: "http://unitsofmeasure.org"\nOBX.6=mg/dL        → unit: "mg/dL"  (no system — not a UCUM-coded unit)' },

            { key: 'obs_code',            src: 'OBX.3',          fhir: 'code',
              label: 'Observation code',
              desc:  'Builds a CodeableConcept from OBX.3. LOINC codes get the standard system URI; local/facility codes are preserved with display text.',
              example: 'OBX.3=2823-3^Potassium^LN → system: "http://loinc.org", code: "2823-3", display: "Potassium"\nOBX.3=GLUC^Glucose^L      → system: "urn:facility:<slug>", code: "GLUC", display: "Glucose"' },

            { key: 'obs_reference_range', src: 'OBX.7',          fhir: 'referenceRange[]',
              label: 'Reference range',
              desc:  'Parses the free-text range string into a structured FHIR referenceRange with typed low/high boundaries.',
              example: 'OBX.7=3.5-5.0   → referenceRange: [{low: {value: 3.5}, high: {value: 5.0}, text: "3.5-5.0"}]\nOBX.7=<10.0     → referenceRange: [{high: {value: 10.0}, text: "<10.0"}]\nOBX.7=(blank)   → field omitted' },

            { key: 'obs_interpretation',  src: 'OBX.8',          fhir: 'interpretation[]',
              label: 'Interpretation flag',
              desc:  'Maps the single-letter HL7 abnormal flag to a FHIR CodeableConcept with display text.',
              example: 'OBX.8=H  → {"code": "H",  "display": "High"}\nOBX.8=L  → {"code": "L",  "display": "Low"}\nOBX.8=AA → {"code": "AA", "display": "Critical"}\nOBX.8=N  → {"code": "N",  "display": "Normal"}' },

            { key: 'obs_status',          src: 'OBX.11',         fhir: 'status',
              label: 'Observation status',
              desc:  'Translates the HL7 result status code to the FHIR observation-status value set.',
              example: 'OBX.11=F → "status": "final"\nOBX.11=P → "status": "preliminary"\nOBX.11=C → "status": "corrected"\nOBX.11=W → "status": "entered-in-error"' },

            { key: 'obs_category',        src: '(fixed)',        fhir: 'category[]',
              label: 'Laboratory category',
              desc:  'Always adds the standard laboratory category so FHIR servers can index and filter Observations correctly.',
              example: '(no HL7 input needed)\n→ "category": [{"coding": [{"code": "laboratory",\n    "system": "http://terminology.hl7.org/CodeSystem/observation-category",\n    "display": "Laboratory"}]}]' },

            { key: 'obs_subject',         src: 'Patient.id',     fhir: 'subject.reference',
              label: 'Subject reference',
              desc:  'Links each Observation back to the Patient resource assembled from the PID segment.',
              example: 'Patient assembled with id="patient-abc123"\n→ "subject": {"reference": "Patient/patient-abc123"}' },

            { key: 'obs_effective',       src: 'OBX.14',         fhir: 'effectiveDateTime',
              label: 'Observation date/time',
              desc:  'Converts the HL7 compact timestamp in OBX.14 to an ISO 8601 datetime string.',
              example: 'OBX.14=20231015143000+0500 → "effectiveDateTime": "2023-10-15T14:30:00+05:00"\nOBX.14=(blank)           → field omitted' },

            { key: 'obs_nte_note',        src: 'NTE.3',           fhir: 'note[].text',
              label: 'NTE comment → note',
              desc:  'Clubs consecutive NTE segments following an OBX into Observation.note[0].text. Multiple NTE lines are joined with newlines; blank NTE lines become paragraph breaks. NTE after OBR (before the first OBX) goes to DiagnosticReport.note.',
              example: 'OBX|2|NM|PT (INR)^INR||1.94|...\nNTE|1||Optimal INR range is 2.0–3.0.\nNTE|2||(blank)\nNTE|3||Studies show low-intensity warfarin...\n→ note: [{text: "Optimal INR range is 2.0–3.0.\\n\\nStudies show low-intensity warfarin..."}]' },
        ];

        const DR_RULES = [
            { key: 'dr_result_links',     src: 'Observation.id[]', fhir: 'result[]',
              label: 'Result references',
              desc:  'Populates DiagnosticReport.result[] with references to every Observation assembled from OBX segments in this message.',
              example: '3 OBX segments assembled\n→ "result": [\n    {"reference": "urn:uuid:obs-1"},\n    {"reference": "urn:uuid:obs-2"},\n    {"reference": "urn:uuid:obs-3"}\n  ]' },

            { key: 'dr_subject',          src: 'Patient.id',       fhir: 'subject.reference',
              label: 'Subject reference',
              desc:  'Links DiagnosticReport.subject to the Patient resource assembled from the PID segment.',
              example: 'Patient id="patient-abc123"\n→ "subject": {"reference": "Patient/patient-abc123"}' },

            { key: 'dr_code',             src: 'OBR.4',            fhir: 'code',
              label: 'Report code',
              desc:  'Builds a CodeableConcept from OBR.4, representing the ordered test or panel.',
              example: 'OBR.4=58410-2^CBC panel^LN\n→ "code": {"coding": [{"code": "58410-2", "system": "http://loinc.org", "display": "CBC panel"}]}' },

            { key: 'dr_status',           src: 'OBR.25',           fhir: 'status',
              label: 'Report status',
              desc:  'Translates the HL7 result status in OBR.25 to the FHIR diagnostic-report-status value set.',
              example: 'OBR.25=F → "status": "final"\nOBR.25=P → "status": "preliminary"\nOBR.25=C → "status": "corrected"' },

            { key: 'dr_effective',        src: 'OBR.7',            fhir: 'effectiveDateTime',
              label: 'Observation date/time',
              desc:  'Converts the specimen collection date/time from OBR.7 (HL7 TS) to ISO 8601.',
              example: 'OBR.7=20231015143000 → "effectiveDateTime": "2023-10-15T14:30:00+00:00"' },

            { key: 'dr_category',         src: '(fixed)',          fhir: 'category[]',
              label: 'Laboratory category',
              desc:  'Always adds the HL7 v2 LAB service category so FHIR servers can differentiate lab reports from imaging or other report types.',
              example: '(no HL7 input needed)\n→ "category": [{"coding": [{"code": "LAB",\n    "system": "http://terminology.hl7.org/CodeSystem/v2-0074",\n    "display": "Laboratory"}]}]' },
        ];

        const esc = (s) => String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');

        // ── Text Report Merging card ──────────────────────────────────────────
        const collapseOff    = rules.collapse_text_obx === false;
        const collapseMode   = collapseOff ? 'never'
                             : (rules.collapse_text_obx_services?.length > 0 ? 'by_service' : 'always');
        const existingSvcs   = (rules.collapse_text_obx_services || []).filter(s => s.trim() !== '');

        // Render existing service tags as chips
        const svcChipsHtml = existingSvcs.map(s =>
            `<span class="merge-svc-chip" data-code="${esc(s.toUpperCase())}"
                style="display:inline-flex;align-items:center;gap:3px;background:#fef3c7;border:1px solid #fcd34d;
                       border-radius:4px;padding:2px 7px;font-size:0.78rem;font-weight:600;color:#78350f;white-space:nowrap;">
                <code style="background:none;font-size:0.78rem;">${esc(s.toUpperCase())}</code>
                <button type="button" class="merge-svc-remove" data-code="${esc(s.toUpperCase())}"
                    style="background:none;border:none;cursor:pointer;color:#b45309;font-size:0.85rem;padding:0;line-height:1;"
                    title="Remove ${esc(s.toUpperCase())}">×</button>
            </span>`
        ).join('');

        const mergeCard = `
        <div style="border:1px solid #d97706;border-radius:8px;overflow:hidden;margin-bottom:1rem;">
            <div style="padding:0.6rem 0.75rem;background:#fef9c3;border-bottom:1px solid #fde68a;display:flex;align-items:center;gap:0.5rem;">
                <i class="fas fa-layer-group" style="color:#b45309;font-size:0.9rem;"></i>
                <span style="font-weight:600;font-size:0.85rem;color:#78350f;">Text Report Merging (TX/FT OBX)</span>
                <span style="font-size:0.75rem;color:#92400e;margin-left:0.25rem;">— collapse HL7 continuation lines into one FHIR Observation</span>
            </div>
            <div style="padding:0.9rem 1rem;">

                <!-- What this does -->
                <div style="background:#fffbeb;border-left:3px solid #f59e0b;border-radius:0 4px 4px 0;padding:0.6rem 0.75rem;margin-bottom:0.85rem;">
                    <p style="font-size:0.8rem;color:#374151;margin:0;line-height:1.6;">
                        Pathology and radiology systems send long free-text reports as <strong>many OBX rows</strong>
                        with the <strong>same OBX.3 code</strong> — one line of text per segment (the HL7 continuation pattern).
                        When merging is <strong>ON</strong>, all those lines are joined into a single
                        <code style="background:#f3f4f6;padding:1px 4px;border-radius:3px;">Observation.valueString</code>
                        — which is what FHIR consumers expect to receive.<br>
                        When merging is <strong>OFF</strong>, each OBX becomes its own Observation — correct for
                        Chemistry/Microbiology panels where every row is a distinct analyte result.
                    </p>
                </div>

                <!-- Side-by-side examples -->
                <div style="display:grid;grid-template-columns:1fr 1fr;gap:0.6rem;margin-bottom:0.9rem;">
                    <div style="background:#f0fdf4;border:1px solid #86efac;border-radius:6px;padding:0.65rem 0.8rem;">
                        <div style="font-size:0.78rem;font-weight:700;color:#15803d;margin-bottom:0.35rem;">
                            ✅ Merge ON — Surgical Path Report
                        </div>
                        <pre style="margin:0;font-size:0.7rem;color:#166534;white-space:pre-wrap;line-height:1.5;background:none;">OBX|1|TX|50398^Surg Path||Diagnosis:
OBX|2|TX|50398^Surg Path||Brain, 4th Ventricle
OBX|3|TX|50398^Surg Path||Mild Gliosis</pre>
                        <div style="margin-top:0.4rem;font-size:0.72rem;color:#166534;border-top:1px solid #bbf7d0;padding-top:0.4rem;">
                            → <strong>1 Observation</strong><br>
                            valueString = <em>"Diagnosis:\nBrain, 4th Ventricle\nMild Gliosis"</em>
                        </div>
                    </div>
                    <div style="background:#fff7ed;border:1px solid #fdba74;border-radius:6px;padding:0.65rem 0.8rem;">
                        <div style="font-size:0.78rem;font-weight:700;color:#c2410c;margin-bottom:0.35rem;">
                            ❌ Merge OFF — Chemistry Panel
                        </div>
                        <pre style="margin:0;font-size:0.7rem;color:#9a3412;white-space:pre-wrap;line-height:1.5;background:none;">OBX|1|NM|2823-3^Potassium||4.2|mmol/L
OBX|2|NM|2951-2^Sodium||138|mmol/L
OBX|3|NM|2075-0^Chloride||102|mmol/L</pre>
                        <div style="margin-top:0.4rem;font-size:0.72rem;color:#9a3412;border-top:1px solid #fed7aa;padding-top:0.4rem;">
                            → <strong>3 Observations</strong> (one per analyte)<br>
                            Each has its own code, value, and unit
                        </div>
                    </div>
                </div>

                <!-- Mode selector -->
                <div style="display:flex;gap:1.5rem;margin-bottom:0.8rem;flex-wrap:wrap;">
                    ${['always','never','by_service'].map(m => {
                        const labels = {
                            always:     'Always merge (all TX/FT OBX)',
                            never:      'Never merge (keep each OBX separate)',
                            by_service: 'Merge only for specific services (OBR.24)'
                        };
                        return `<label style="display:flex;align-items:center;gap:0.4rem;cursor:pointer;font-size:0.83rem;color:#374151;">
                            <input type="radio" name="collapse_text_obx_mode" value="${m}"
                                ${collapseMode === m ? 'checked' : ''}
                                ${!masterOn ? 'disabled' : ''}
                                style="accent-color:#b45309;cursor:pointer;">
                            ${labels[m]}
                        </label>`;
                    }).join('')}
                </div>

                <!-- Service tag input (shown only in by_service mode) -->
                <div id="collapse-services-section"
                     style="display:${collapseMode === 'by_service' ? 'block' : 'none'};
                            background:#fffbeb;border:1px solid #fde68a;border-radius:6px;padding:0.75rem;">
                    <p style="font-size:0.78rem;color:#78350f;margin:0 0 0.5rem;font-weight:600;">
                        Merge only when OBR.24 (Diagnostic Service) matches one of these codes:
                    </p>

                    <!-- Tag chips + text input -->
                    <div id="merge-svc-tags"
                         style="display:flex;flex-wrap:wrap;align-items:center;gap:0.35rem;
                                min-height:32px;background:#fff;border:1px solid #fcd34d;border-radius:5px;
                                padding:4px 6px;cursor:text;">
                        ${svcChipsHtml}
                        <input id="merge-svc-input" type="text" placeholder="Type a code and press Enter (e.g. AP, RAD, SP…)"
                            ${!masterOn ? 'disabled' : ''}
                            style="border:none;outline:none;font-size:0.8rem;min-width:200px;flex:1;
                                   background:transparent;color:#374151;padding:2px 0;">
                    </div>

                    <!-- Quick-add hint chips -->
                    <div style="margin-top:0.45rem;font-size:0.74rem;color:#92400e;">
                        Common codes — click to add:
                        ${['AP','SP','RAD','NM','NMR','OTH','CH','MB','LAB'].map(c =>
                            `<button type="button" class="merge-svc-hint" data-code="${c}"
                                style="background:#fef3c7;border:1px solid #fcd34d;border-radius:3px;
                                       padding:1px 6px;font-size:0.74rem;font-weight:600;color:#78350f;
                                       cursor:pointer;margin:0 2px;">${c}</button>`
                        ).join('')}
                    </div>

                    <p style="font-size:0.74rem;color:#6b7280;margin:0.5rem 0 0;">
                        <strong>AP</strong> = Anatomic Pathology &nbsp;·&nbsp;
                        <strong>SP</strong> = Surgical Pathology &nbsp;·&nbsp;
                        <strong>RAD</strong> = Radiology &nbsp;·&nbsp;
                        <strong>NM</strong> = Nuclear Medicine &nbsp;·&nbsp;
                        <strong>CH</strong> = Chemistry &nbsp;·&nbsp;
                        <strong>MB</strong> = Microbiology &nbsp;·&nbsp;
                        <strong>LAB</strong> = General Lab<br>
                        Any OBR.24 value not in this list keeps each OBX as a separate Observation.
                        If OBR.24 is blank, merging is skipped.
                    </p>
                </div>

            </div>
        </div>`;

        const ruleRow = (r) => {
            const on = masterOn && isEnabled(r.key);
            const exampleHtml = r.example ? `
                <details style="margin-top:5px;">
                    <summary style="font-size:0.72rem;color:#4f46e5;cursor:pointer;user-select:none;list-style:none;display:inline-flex;align-items:center;gap:3px;outline:none;">
                        <span style="font-size:0.65rem;">▸</span> Example
                    </summary>
                    <pre style="margin:4px 0 0;font-size:0.7rem;line-height:1.55;color:#1e3a8a;
                                background:#eff6ff;border:1px solid #bfdbfe;border-radius:4px;
                                padding:5px 7px;white-space:pre-wrap;overflow-x:auto;">${esc(r.example)}</pre>
                </details>` : '';
            return `
            <tr style="border-bottom: 1px solid #f1f5f9; ${!on ? 'opacity:0.45;' : ''}">
                <td style="padding: 0.6rem 0.75rem; white-space: nowrap; vertical-align:top;">
                    <code style="background:#dbeafe;padding:2px 6px;border-radius:3px;color:#1e3a8a;font-size:0.78rem;">${esc(r.src)}</code>
                </td>
                <td style="padding: 0.6rem 0.75rem; white-space: nowrap; vertical-align:top;">
                    <code style="background:#fce7f3;padding:2px 6px;border-radius:3px;color:#831843;font-size:0.78rem;">${esc(r.fhir)}</code>
                </td>
                <td style="padding: 0.6rem 0.75rem; vertical-align:top;">
                    <div style="font-size:0.82rem;font-weight:600;color:#1e293b;">${esc(r.label)}</div>
                    <div style="font-size:0.75rem;color:#64748b;margin-top:2px;">${esc(r.desc)}</div>
                    ${exampleHtml}
                </td>
                <td style="padding: 0.6rem 0.75rem; text-align: center; vertical-align:top;">
                    <label style="cursor:pointer;display:inline-flex;align-items:center;" title="${on ? 'Disable this assembly rule' : 'Enable this assembly rule'}">
                        <input type="checkbox" class="assembly-rule-toggle"
                            data-rule-key="${esc(r.key)}"
                            ${isEnabled(r.key) ? 'checked' : ''}
                            ${!masterOn ? 'disabled' : ''}
                            style="width:15px;height:15px;cursor:pointer;accent-color:#1e3a8a;">
                    </label>
                </td>
            </tr>`;
        };

        return `
            <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:0.75rem;">
                <div style="font-size:0.8rem;color:#64748b;">
                    OOB certified rules — disable any that conflict with your interface data.
                    Add a <strong>Script Enrichment</strong> step after this one to extend.
                </div>
                <label style="display:flex;align-items:center;gap:0.4rem;cursor:pointer;white-space:nowrap;">
                    <input type="checkbox" name="config_assembleObservations" id="assemblyMasterToggle"
                        ${masterOn ? 'checked' : ''}
                        style="width:15px;height:15px;cursor:pointer;accent-color:#1e3a8a;">
                    <span style="font-size:0.82rem;font-weight:600;color:#374151;">Assembly on</span>
                </label>
            </div>

            ${mergeCard}

            ${this._renderAttachmentEncodingCard(step, rules, masterOn, esc)}

            <!-- Observation rules -->
            <div style="border:1px solid #e5e7eb;border-radius:8px;overflow:hidden;margin-bottom:1rem;">
                <div style="padding:0.6rem 0.75rem;background:#f8fafc;border-bottom:1px solid #e5e7eb;display:flex;align-items:center;gap:0.5rem;">
                    <i class="fas fa-microscope" style="color:#7c3aed;font-size:0.9rem;"></i>
                    <span style="font-weight:600;font-size:0.85rem;color:#1e293b;">Observation rules</span>
                    <span style="font-size:0.75rem;color:#64748b;margin-left:0.25rem;">— one row per OBX segment</span>
                </div>
                <table style="width:100%;border-collapse:collapse;">
                    <thead style="background:#fafafa;">
                        <tr>
                            <th style="padding:0.5rem 0.75rem;text-align:left;font-size:0.75rem;color:#64748b;border-bottom:1px solid #f1f5f9;white-space:nowrap;">HL7 Source</th>
                            <th style="padding:0.5rem 0.75rem;text-align:left;font-size:0.75rem;color:#64748b;border-bottom:1px solid #f1f5f9;white-space:nowrap;">FHIR Target</th>
                            <th style="padding:0.5rem 0.75rem;text-align:left;font-size:0.75rem;color:#64748b;border-bottom:1px solid #f1f5f9;">Transform</th>
                            <th style="padding:0.5rem 0.75rem;text-align:center;font-size:0.75rem;color:#64748b;border-bottom:1px solid #f1f5f9;width:60px;">On</th>
                        </tr>
                    </thead>
                    <tbody>${OBS_RULES.map(ruleRow).join('')}</tbody>
                </table>
            </div>

            <!-- DiagnosticReport rules -->
            <div style="border:1px solid #e5e7eb;border-radius:8px;overflow:hidden;margin-bottom:0.75rem;">
                <div style="padding:0.6rem 0.75rem;background:#f8fafc;border-bottom:1px solid #e5e7eb;display:flex;align-items:center;gap:0.5rem;">
                    <i class="fas fa-file-medical-alt" style="color:#0284c7;font-size:0.9rem;"></i>
                    <span style="font-weight:600;font-size:0.85rem;color:#1e293b;">DiagnosticReport rules</span>
                    <span style="font-size:0.75rem;color:#64748b;margin-left:0.25rem;">— applied once per message</span>
                </div>
                <table style="width:100%;border-collapse:collapse;">
                    <thead style="background:#fafafa;">
                        <tr>
                            <th style="padding:0.5rem 0.75rem;text-align:left;font-size:0.75rem;color:#64748b;border-bottom:1px solid #f1f5f9;white-space:nowrap;">HL7 Source</th>
                            <th style="padding:0.5rem 0.75rem;text-align:left;font-size:0.75rem;color:#64748b;border-bottom:1px solid #f1f5f9;white-space:nowrap;">FHIR Target</th>
                            <th style="padding:0.5rem 0.75rem;text-align:left;font-size:0.75rem;color:#64748b;border-bottom:1px solid #f1f5f9;">Transform</th>
                            <th style="padding:0.5rem 0.75rem;text-align:center;font-size:0.75rem;color:#64748b;border-bottom:1px solid #f1f5f9;width:60px;">On</th>
                        </tr>
                    </thead>
                    <tbody>${DR_RULES.map(ruleRow).join('')}</tbody>
                </table>
            </div>

            ${this._renderOptionalSegmentBlocks(step, rules, esc)}
        `;
    }

    /**
     * Render the Attachment Encoding card — a single user-friendly switch that
     * controls whether base64 data in OBX ED (Encapsulated Data) segments is
     * validated before being placed in FHIR Attachment.data.
     *
     * Maps to assemblyRules.validate_base64 (default ON = validate).
     */
    _renderAttachmentEncodingCard(step, rules, masterOn, esc) {
        // validate_base64 defaults ON (undefined → checked).  Only explicit false = off.
        const validateOn = rules.validate_base64 !== false;

        return `
        <div style="border:1px solid #0891b2;border-radius:8px;overflow:hidden;margin-bottom:1rem;">
            <div style="padding:0.6rem 0.75rem;background:#ecfeff;border-bottom:1px solid #a5f3fc;display:flex;align-items:center;gap:0.5rem;">
                <i class="fas fa-paperclip" style="color:#0e7490;font-size:0.9rem;"></i>
                <span style="font-weight:600;font-size:0.85rem;color:#164e63;">Attachment Encoding</span>
                <span style="font-size:0.75rem;color:#0e7490;margin-left:0.25rem;">— PDF, image and binary files carried in OBX (ED type)</span>
            </div>
            <div style="padding:0.9rem 1rem;">

                <div style="background:#f0fdff;border-left:3px solid #22d3ee;border-radius:0 4px 4px 0;padding:0.6rem 0.75rem;margin-bottom:0.85rem;">
                    <p style="font-size:0.8rem;color:#374151;margin:0;line-height:1.6;">
                        When an HL7 message carries a <strong>PDF, image, or other binary file</strong> inside an
                        <code style="background:#f3f4f6;padding:1px 4px;border-radius:3px;">OBX</code> segment
                        (data type&nbsp;<strong>ED</strong>), that file is encoded in
                        <a href="https://en.wikipedia.org/wiki/Base64" target="_blank"
                           style="color:#0e7490;">base64</a> and placed into
                        <code style="background:#f3f4f6;padding:1px 4px;border-radius:3px;">Attachment.data</code>
                        in the FHIR output.<br><br>
                        <strong>Check encoding&nbsp;ON&nbsp;(recommended):</strong> the engine verifies the data can be
                        decoded before writing it. If a sending system accidentally truncates or corrupts the file data,
                        the attachment is flagged with a warning title instead of creating an invalid FHIR resource.<br><br>
                        <strong>Check encoding&nbsp;OFF:</strong> the raw value is copied straight through — choose this
                        only when you are certain the source system always sends clean base64 and you need to avoid
                        any processing overhead.
                    </p>
                </div>

                <!-- Side-by-side outcome preview -->
                <div style="display:grid;grid-template-columns:1fr 1fr;gap:0.6rem;margin-bottom:0.9rem;">
                    <div style="background:#f0fdf4;border:1px solid #86efac;border-radius:6px;padding:0.65rem 0.8rem;">
                        <div style="font-size:0.78rem;font-weight:700;color:#15803d;margin-bottom:0.35rem;">
                            ✅ Check ON — corrupt data detected
                        </div>
                        <pre style="margin:0;font-size:0.7rem;color:#166534;white-space:pre-wrap;line-height:1.5;background:none;">OBX|1|ED|PDF^Report||^PDF^Base64^[bad data]
→ Attachment.title = "(data omitted — not valid base64)"
→ FHIR validator: PASS  ✓</pre>
                    </div>
                    <div style="background:#fff7ed;border:1px solid #fdba74;border-radius:6px;padding:0.65rem 0.8rem;">
                        <div style="font-size:0.78rem;font-weight:700;color:#c2410c;margin-bottom:0.35rem;">
                            ❌ Check OFF — corrupt data passes through
                        </div>
                        <pre style="margin:0;font-size:0.7rem;color:#9a3412;white-space:pre-wrap;line-height:1.5;background:none;">OBX|1|ED|PDF^Report||^PDF^Base64^[bad data]
→ Attachment.data = "[bad data]"
→ FHIR validator: FAIL  ✗  (att-1)</pre>
                    </div>
                </div>

                <!-- Toggle row -->
                <div style="display:flex;align-items:center;justify-content:space-between;
                            background:#f8fdfe;border:1px solid #a5f3fc;border-radius:6px;
                            padding:0.65rem 0.9rem;">
                    <div>
                        <div style="font-size:0.83rem;font-weight:600;color:#164e63;">
                            Check attachment encoding before writing to FHIR
                        </div>
                        <div style="font-size:0.76rem;color:#0e7490;margin-top:2px;">
                            Recommended — prevents invalid base64 from reaching your FHIR server
                        </div>
                    </div>
                    <label style="display:flex;align-items:center;gap:0.4rem;cursor:pointer;white-space:nowrap;margin-left:1rem;"
                           title="${validateOn ? 'Disable encoding check (passthrough)' : 'Enable encoding check (recommended)'}">
                        <input type="checkbox" class="assembly-rule-toggle"
                            data-rule-key="validate_base64"
                            ${validateOn ? 'checked' : ''}
                            ${!masterOn ? 'disabled' : ''}
                            style="width:15px;height:15px;cursor:pointer;accent-color:#0891b2;">
                        <span data-validate-base64-label
                              style="font-size:0.82rem;font-weight:600;color:#374151;">
                            ${validateOn ? 'Check encoding' : 'Pass through (no check)'}
                        </span>
                    </label>
                </div>

            </div>
        </div>`;
    }

    /**
     * Render Optional Output Resources section of the Assembly tab.
     * These are OFF by default — opposite of the OBS/DR rules above.
     * Each toggle writes to step.config.assemblyRules["opt_*"] via the
     * existing .assembly-rule-toggle event listener.
     */
    _renderOptionalSegmentBlocks(step, rules, esc) {
        const messageType = this.builder?.pipeline?.messageType || step.config?.message_type || '';

        // Registry of optional blocks. AppliesTo: event codes after "^".
        const OPT_BLOCKS = [
            {
                key: 'opt_PV1_Encounter',
                label: 'Encounter from PV1',
                src: 'PV1',
                fhir: 'Encounter',
                desc: 'Assembles PV1 visit information (class, period, location, attending) into an Encounter resource. PV1 is present in ORU messages but Encounter is not produced by default.',
                appliesTo: ['R01', 'T01', 'T02', 'T11'],
            },
            {
                key: 'opt_SPM_Specimen',
                label: 'Specimen from SPM',
                src: 'SPM',
                fhir: 'Specimen',
                desc: 'Assembles SPM segment fields (type, collection date, body site) into a Specimen resource linked to DiagnosticReport.',
                appliesTo: ['R01'],
            },
            {
                key: 'opt_ORC_OrderingPractitioner',
                label: 'Ordering Provider from ORC',
                src: 'ORC.12',
                fhir: 'Practitioner',
                desc: 'Adds the ordering provider (ORC.12) as a Practitioner and links it to DiagnosticReport.basedOn.',
                appliesTo: ['R01', 'O01', 'O19'],
            },
        ];

        // Filter to blocks that apply to the current message type.
        const eventCode = messageType.includes('^') ? messageType.split('^')[1] : messageType;
        const applicable = OPT_BLOCKS.filter(b =>
            b.appliesTo.length === 0 || b.appliesTo.includes(eventCode)
        );

        if (applicable.length === 0) return '';

        const optRows = applicable.map(b => {
            // Optional blocks default OFF: only checked when rules[key] === true explicitly.
            const isOn = rules[b.key] === true;
            return `<tr style="border-bottom:1px solid #f1f5f9;opacity:${isOn ? '1' : '0.6'}">
                <td style="padding:0.6rem 0.75rem;white-space:nowrap;vertical-align:top;">
                    <code style="background:#dbeafe;padding:2px 6px;border-radius:3px;color:#1e3a8a;font-size:0.78rem;">${esc(b.src)}</code>
                </td>
                <td style="padding:0.6rem 0.75rem;white-space:nowrap;vertical-align:top;">
                    <code style="background:#fce7f3;padding:2px 6px;border-radius:3px;color:#831843;font-size:0.78rem;">${esc(b.fhir)}</code>
                </td>
                <td style="padding:0.6rem 0.75rem;vertical-align:top;">
                    <div style="font-size:0.82rem;font-weight:600;color:#1e293b;">${esc(b.label)}</div>
                    <div style="font-size:0.75rem;color:#64748b;margin-top:2px;">${esc(b.desc)}</div>
                </td>
                <td style="padding:0.6rem 0.75rem;text-align:center;vertical-align:top;">
                    <label style="cursor:pointer;display:inline-flex;align-items:center;" title="${isOn ? 'Disable' : 'Enable'} this optional resource">
                        <input type="checkbox" class="assembly-rule-toggle"
                            data-rule-key="${esc(b.key)}"
                            ${isOn ? 'checked' : ''}
                            style="width:15px;height:15px;cursor:pointer;accent-color:#16a34a;">
                    </label>
                </td>
            </tr>`;
        }).join('');

        return `
            <!-- Optional output resources — OFF by default -->
            <div style="border:1px solid #bbf7d0;border-radius:8px;overflow:hidden;margin-top:1rem;">
                <div style="padding:0.6rem 0.75rem;background:#f0fdf4;border-bottom:1px solid #bbf7d0;display:flex;align-items:center;gap:0.5rem;">
                    <i class="fas fa-plus-circle" style="color:#16a34a;font-size:0.9rem;"></i>
                    <span style="font-weight:600;font-size:0.85rem;color:#14532d;">Optional Output Resources</span>
                    <span style="font-size:0.75rem;color:#166534;margin-left:0.25rem;">— off by default, enable what your downstream system requires</span>
                </div>
                <table style="width:100%;border-collapse:collapse;">
                    <thead style="background:#f0fdf4;">
                        <tr>
                            <th style="padding:0.5rem 0.75rem;text-align:left;font-size:0.75rem;color:#166534;border-bottom:1px solid #dcfce7;white-space:nowrap;">HL7 Source</th>
                            <th style="padding:0.5rem 0.75rem;text-align:left;font-size:0.75rem;color:#166534;border-bottom:1px solid #dcfce7;white-space:nowrap;">FHIR Resource</th>
                            <th style="padding:0.5rem 0.75rem;text-align:left;font-size:0.75rem;color:#166534;border-bottom:1px solid #dcfce7;">What it produces</th>
                            <th style="padding:0.5rem 0.75rem;text-align:center;font-size:0.75rem;color:#166534;border-bottom:1px solid #dcfce7;width:60px;">On</th>
                        </tr>
                    </thead>
                    <tbody>${optRows}</tbody>
                </table>
            </div>
        `;
    }

    /**
     * Extract FHIR resource type from a mapping's fhirPath.
     * "Patient.name[0].family" → "Patient"
     * "["enrichment"].data.x"  → "Enriched Data"
     * "" / undefined           → "Other"
     */
    _mappingResourceType(mapping) {
        // Prefer the explicit resourceType field set by the backend.
        const explicit = mapping.resourceType || mapping.fhirResourceType;
        if (explicit) return explicit;

        // Fallback: parse the leading segment from a fully-qualified FHIR path.
        const path = mapping.fhirPath || mapping.targetField || mapping.targetPath || '';
        if (!path) return 'Other';
        if (path.startsWith('[')) return 'Enriched Data';
        const dot = path.indexOf('.');
        const bracket = path.indexOf('[');
        let end = path.length;
        if (dot > 0) end = Math.min(end, dot);
        if (bracket > 0) end = Math.min(end, bracket);
        const rt = path.slice(0, end).trim();
        // Only use the parsed segment if it looks like a FHIR resource type (PascalCase).
        return /^[A-Z][A-Za-z]+$/.test(rt) ? rt : 'Other';
    }

    // Icon + accent colour per resource type
    _resourceTypeStyle(rt) {
        const MAP = {
            Patient:           { icon: 'fa-user',               color: '#1e3a8a', bg: '#dbeafe' },
            Encounter:         { icon: 'fa-hospital',            color: '#065f46', bg: '#d1fae5' },
            Observation:       { icon: 'fa-microscope',          color: '#6d28d9', bg: '#ede9fe' },
            DiagnosticReport:  { icon: 'fa-file-medical-alt',    color: '#0369a1', bg: '#e0f2fe' },
            Practitioner:      { icon: 'fa-user-md',             color: '#92400e', bg: '#fef3c7' },
            Organization:      { icon: 'fa-building',            color: '#374151', bg: '#f3f4f6' },
            MessageHeader:     { icon: 'fa-envelope',            color: '#7c3aed', bg: '#f5f3ff' },
            Coverage:          { icon: 'fa-id-card',             color: '#0f766e', bg: '#ccfbf1' },
            Condition:         { icon: 'fa-heartbeat',           color: '#b91c1c', bg: '#fee2e2' },
            Procedure:         { icon: 'fa-stethoscope',         color: '#047857', bg: '#d1fae5' },
            MedicationRequest: { icon: 'fa-pills',               color: '#7c2d12', bg: '#ffedd5' },
            AllergyIntolerance:{ icon: 'fa-allergies',           color: '#92400e', bg: '#fef3c7' },
            ServiceRequest:    { icon: 'fa-clipboard-list',      color: '#1d4ed8', bg: '#dbeafe' },
            'Enriched Data':   { icon: 'fa-database',            color: '#059669', bg: '#d1fae5' },
        };
        return MAP[rt] || { icon: 'fa-cube', color: '#475569', bg: '#f1f5f9' };
    }

    /**
     * Render mapping table grouped by FHIR resource type.
     */
    renderMappingTable(mappings) {
        if (!mappings || mappings.length === 0) {
            return `
                <div style="padding: 2rem; text-align: center; color: #94a3b8;">
                    <i class="fas fa-inbox" style="font-size: 3rem; margin-bottom: 1rem; opacity: 0.5;"></i>
                    <p>No mappings configured</p>
                    <p style="font-size: 0.875rem;">Click "Add Mapping" to create your first mapping</p>
                </div>
            `;
        }

        // Group mappings preserving original indices
        const groups = new Map(); // resourceType → [{mapping, originalIndex}]
        mappings.forEach((mapping, index) => {
            const rt = this._mappingResourceType(mapping);
            if (!groups.has(rt)) groups.set(rt, []);
            groups.get(rt).push({ mapping, index });
        });

        const renderRow = ({ mapping, index }) => {
            // ── Normalise FHIR path to always show resource-qualified form ──────
            // OOB mappings carry resourceType + targetPath separately;
            // user-added mappings carry the full path in fhirPath.
            const rawFhir      = mapping.fhirPath || mapping.targetField || mapping.targetPath || 'N/A';
            const rt           = mapping.resourceType || mapping.fhirResourceType || '';
            const fhirPath     = (rt && rawFhir !== 'N/A' && !rawFhir.startsWith(rt + '.'))
                ? `${rt}.${rawFhir}`
                : rawFhir;

            const transformType = mapping.transformType || mapping.dataTypeTransform || '';
            const isStatic     = transformType === 'static_value';
            const staticVal    = mapping.staticValue || mapping.defaultValue || '';

            // Source column: show HL7 field or a "Static" indicator
            const hl7FieldRaw  = mapping.hl7Field || mapping.sourceField || mapping.sourcePath || '';
            const hl7Field     = isStatic
                ? `<em style="color:#059669;font-size:0.78rem;">static: "${staticVal}"</em>`
                : `<code style="background:#dbeafe;padding:2px 7px;border-radius:3px;color:#1e3a8a;font-weight:500;font-size:0.82rem;">${hl7FieldRaw || 'N/A'}</code>`;

            // Transform badge — inlined so it works on pipeline-builder.html
            // regardless of whether WizardView.js is loaded.
            const TRANSFORM_LABELS = {
                'ce_to_codeableconcept':           'CE → Code',
                'cx_to_identifier':                'CX → ID',
                'xpn_to_humanname':                'XPN → Name',
                'xad_to_address':                  'XAD → Addr',
                'xtn_to_contactpoint':             'XTN → Contact',
                'ts_to_datetime':                  'TS → DateTime',
                'ts_to_date':                      'TS → Date',
                'gender_mapping':                  'Gender',
                'administrative_sex':              'Gender',
                'msh9_trigger_event_to_coding':    'Event Coding',
                'obx_value_by_type':               'OBX Value',
                'hl7_active_flag':                 'Y/N → Bool',
                'static_value':                    'Static',
            };
            let transformBadge = '';
            if (isStatic) {
                transformBadge = `<span style="display:inline-block;padding:2px 6px;border-radius:10px;font-size:10px;font-weight:600;background:#dcfce7;color:#166534;border:1px solid #bbf7d0;white-space:nowrap;">Static</span>`;
            } else if (transformType) {
                const label = TRANSFORM_LABELS[transformType] || transformType;
                transformBadge = `<span style="display:inline-block;padding:2px 6px;border-radius:10px;font-size:10px;font-weight:600;background:#eff6ff;color:#2563eb;border:1px solid #bfdbfe;white-space:nowrap;max-width:90px;overflow:hidden;text-overflow:ellipsis;" title="${transformType}">${label}</span>`;
            } else {
                transformBadge = `<span style="display:inline-block;padding:2px 6px;border-radius:10px;font-size:10px;font-weight:500;background:#f3f4f6;color:#9ca3af;border:1px solid #e5e7eb;white-space:nowrap;" title="Auto-translate based on HL7 and FHIR data types">Auto</span>`;
            }
            return `
                <tr class="mapping-row" data-index="${index}"
                    style="border-bottom: 1px solid #f1f5f9; cursor: pointer; transition: background 0.15s;"
                    onclick="window.propertiesPanel.editMapping(${index})"
                    onmouseover="this.style.background='#f0f9ff'"
                    onmouseout="this.style.background=''">
                    <td style="padding: 0.6rem 0.75rem;">${hl7Field}</td>
                    <td style="padding: 0.6rem 0.75rem;">
                        <code style="background: #fce7f3; padding: 2px 7px; border-radius: 3px; color: #831843; font-weight: 500; font-size: 0.82rem;">${fhirPath}</code>
                    </td>
                    <td style="padding: 0.6rem 0.75rem; color: #64748b; font-size: 0.82rem;">
                        ${transformBadge}
                    </td>
                    <td style="padding: 0.6rem 0.75rem; text-align: center; white-space: nowrap;">
                        <button class="edit-mapping-btn" data-index="${index}"
                            onclick="event.stopPropagation(); window.propertiesPanel.editMapping(${index})"
                            style="background:none;border:none;color:#2563eb;cursor:pointer;padding:3px 6px;"
                            onmouseover="this.style.color='#1d4ed8'"
                            onmouseout="this.style.color='#2563eb'"
                            title="Edit mapping">
                            <i class="fas fa-pencil-alt" style="font-size:0.8rem;"></i>
                        </button>
                        <button class="delete-mapping-btn" data-index="${index}"
                            onclick="event.stopPropagation(); window.propertiesPanel.deleteMapping(${index})"
                            style="background:none;border:none;color:#dc2626;cursor:pointer;padding:3px 6px;"
                            onmouseover="this.style.color='#991b1b'"
                            onmouseout="this.style.color='#dc2626'"
                            title="Delete mapping">
                            <i class="fas fa-trash" style="font-size:0.8rem;"></i>
                        </button>
                    </td>
                </tr>`;
        };

        const groupBlocks = [];
        groups.forEach((rows, rt) => {
            const style = this._resourceTypeStyle(rt);
            groupBlocks.push(`
                <div style="margin-bottom: 0.75rem; border: 1px solid #e5e7eb; border-radius: 8px; overflow: hidden;">
                    <div style="display:flex; align-items:center; justify-content:space-between;
                                padding: 0.55rem 0.75rem; background: ${style.bg};
                                border-bottom: 1px solid #e5e7eb; cursor:pointer; user-select:none;"
                         onclick="this.nextElementSibling.style.display = this.nextElementSibling.style.display==='none' ? '' : 'none';
                                  this.querySelector('.grp-chevron').style.transform = this.nextElementSibling.style.display==='' ? 'rotate(0deg)' : 'rotate(-90deg)';">
                        <span style="display:flex; align-items:center; gap:0.5rem;">
                            <i class="fas ${style.icon}" style="color:${style.color}; font-size:0.85rem;"></i>
                            <span style="font-weight:600; font-size:0.85rem; color:${style.color};">${rt}</span>
                            <span style="font-size:0.75rem; color:#64748b; font-weight:400;">${rows.length} mapping${rows.length !== 1 ? 's' : ''}</span>
                        </span>
                        <i class="fas fa-chevron-down grp-chevron" style="font-size:0.75rem; color:${style.color}; transition:transform 0.2s;"></i>
                    </div>
                    <div>
                        <table style="width:100%; border-collapse:collapse; font-size:0.875rem;">
                            <thead style="background:#fafafa;">
                                <tr>
                                    <th style="padding:0.45rem 0.75rem; text-align:left; border-bottom:1px solid #f1f5f9; font-weight:600; font-size:0.78rem; color:#64748b;">HL7 Field</th>
                                    <th style="padding:0.45rem 0.75rem; text-align:left; border-bottom:1px solid #f1f5f9; font-weight:600; font-size:0.78rem; color:#64748b;">FHIR Path</th>
                                    <th style="padding:0.45rem 0.75rem; text-align:left; border-bottom:1px solid #f1f5f9; font-weight:600; font-size:0.78rem; color:#64748b;">Data Type</th>
                                    <th style="padding:0.45rem 0.75rem; text-align:center; border-bottom:1px solid #f1f5f9; font-weight:600; font-size:0.78rem; color:#64748b; width:50px;"></th>
                                </tr>
                            </thead>
                            <tbody>${rows.map(renderRow).join('')}</tbody>
                        </table>
                    </div>
                </div>`);
        });

        return groupBlocks.join('');
    }

    /**
     * Create script editor section
     */
    createScriptSection(step) {
        return `
            <div class="form-section">
                <h4>Custom Script</h4>

                <div class="form-group">
                    <label>Script Type</label>
                    <select id="scriptType">
                        <option value="javascript" ${step.scriptType === 'javascript' ? 'selected' : ''}>JavaScript</option>
                        <option value="lua" ${step.scriptType === 'lua' ? 'selected' : ''}>Lua</option>
                    </select>
                </div>

                <div class="form-group">
                    <label>Script Code</label>
                    <textarea id="scriptContent" rows="15" style="font-family: 'Courier New', monospace;">${step.scriptContent || ''}</textarea>
                    <div style="margin-top: 8px; display: flex; gap: 10px; align-items: center;">
                        <button type="button" id="validateScriptBtn" class="btn-secondary" style="padding: 6px 12px;">
                            🔍 Validate Script
                        </button>
                        <div id="scriptValidationResult" style="flex: 1;"></div>
                    </div>
                    <small style="color: #64748b; display: block; margin-top: 8px;">
                        <strong>Read:</strong> <code>input</code> — full pipeline message data<br>
                        <strong>Write (option A):</strong> assign to <code>output</code> — <code>output.patientId = "123";</code><br>
                        <strong>Write (option B):</strong> return an object — <code>return { patientId: "123" };</code><br>
                        <strong>Write (option C):</strong> end with an expression — <code>({ patientId: "123" })</code>
                    </small>
                </div>
            </div>
        `;
    }

    /**
     * Attach form event listeners
     */
    attachFormEvents(form, step, isPreview = false) {
        // Save button (for editing existing steps)
        const saveBtn = form.querySelector('#saveStepBtn');
        if (saveBtn) {
            saveBtn.addEventListener('click', () => this.saveStepProperties(step));
        }

        // Add to Pipeline button (for preview mode)
        const addBtn = form.querySelector('#addToPipelineBtn');
        if (addBtn) {
            addBtn.addEventListener('click', () => this.addStepToPipeline(step));
        }

        // Cancel button
        const cancelBtn = form.querySelector('#cancelStepBtn');
        if (cancelBtn) {
            cancelBtn.addEventListener('click', () => this.closeModal());
        }

        // Real-time validation
        const configTextarea = form.querySelector('#stepConfig');
        if (configTextarea) {
            configTextarea.addEventListener('blur', () => this.validateJSON(configTextarea));
        }

        // Script validation button
        const validateScriptBtn = form.querySelector('#validateScriptBtn');
        if (validateScriptBtn) {
            validateScriptBtn.addEventListener('click', () => this.validateScript(step));
        }

        // Icon is now automatically assigned based on step type - no manual input needed

        // === HL7→FHIR Mapping Tab Switching ===
        const configTabs = form.querySelectorAll('.config-tab');
        configTabs.forEach(tab => {
            tab.addEventListener('click', (e) => {
                const targetTab = e.currentTarget.dataset.tab;

                // Update tab styles
                configTabs.forEach(t => {
                    t.classList.remove('active');
                    t.style.borderBottom = 'none';
                    t.style.color = '#64748b';
                    t.style.fontWeight = '500';
                });
                e.currentTarget.classList.add('active');
                e.currentTarget.style.borderBottom = '3px solid #1e3a8a';
                e.currentTarget.style.color = '#1e3a8a';
                e.currentTarget.style.fontWeight = '600';

                // Show corresponding content
                form.querySelectorAll('.config-tab-content').forEach(content => {
                    content.style.display = 'none';
                });
                const targetContent = form.querySelector(`[data-tab-content="${targetTab}"]`);
                if (targetContent) {
                    targetContent.style.display = 'block';
                }
            });
        });

        // === Assembly tab: master toggle dims/enables individual rule rows ===
        const assemblyMaster = form.querySelector('#assemblyMasterToggle');
        if (assemblyMaster) {
            assemblyMaster.addEventListener('change', () => {
                const ruleRows = form.querySelectorAll('tr[style*="border-bottom"]');
                const toggles = form.querySelectorAll('.assembly-rule-toggle');
                const on = assemblyMaster.checked;
                toggles.forEach(t => { t.disabled = !on; });
                // dim entire rows when master is off
                form.querySelectorAll('.assembly-rule-toggle').forEach(t => {
                    const row = t.closest('tr');
                    if (row) row.style.opacity = on ? '1' : '0.45';
                });
            });
        }

        // === Assembly tab: individual rule toggles write to step.config.assemblyRules ===
        form.querySelectorAll('.assembly-rule-toggle').forEach(toggle => {
            toggle.addEventListener('change', () => {
                step.config = step.config || {};
                step.config.assemblyRules = step.config.assemblyRules || {};
                step.config.assemblyRules[toggle.dataset.ruleKey] = toggle.checked;
                const row = toggle.closest('tr');
                if (row) row.style.opacity = toggle.checked ? '1' : '0.45';
                // Update the validate_base64 label text
                if (toggle.dataset.ruleKey === 'validate_base64') {
                    const lbl = toggle.parentElement.querySelector('[data-validate-base64-label]');
                    if (lbl) lbl.textContent = toggle.checked ? 'Check encoding' : 'Pass through (no check)';
                }
            });
        });

        // === Text Report Merging card ===
        const getTagsFromDOM = () => {
            return [...form.querySelectorAll('.merge-svc-chip')].map(el => el.dataset.code);
        };

        const saveMergeConfig = () => {
            const modeEl = form.querySelector('input[name="collapse_text_obx_mode"]:checked');
            if (!modeEl) return;
            const mode = modeEl.value;
            step.config = step.config || {};
            step.config.assemblyRules = step.config.assemblyRules || {};
            const ar = step.config.assemblyRules;
            if (mode === 'never') {
                ar.collapse_text_obx = false;
                ar.collapse_text_obx_services = [];
            } else if (mode === 'by_service') {
                ar.collapse_text_obx = true;
                ar.collapse_text_obx_services = getTagsFromDOM();
            } else { // always
                ar.collapse_text_obx = true;
                ar.collapse_text_obx_services = [];
            }
        };

        const addServiceTag = (code) => {
            const cleaned = code.trim().toUpperCase();
            if (!cleaned) return;
            const tagsContainer = form.querySelector('#merge-svc-tags');
            if (!tagsContainer) return;
            // Dedup
            if (form.querySelector(`.merge-svc-chip[data-code="${cleaned}"]`)) return;
            const chip = document.createElement('span');
            chip.className = 'merge-svc-chip';
            chip.dataset.code = cleaned;
            chip.style.cssText = 'display:inline-flex;align-items:center;gap:3px;background:#fef3c7;border:1px solid #fcd34d;border-radius:4px;padding:2px 7px;font-size:0.78rem;font-weight:600;color:#78350f;white-space:nowrap;';
            chip.innerHTML = `<code style="background:none;font-size:0.78rem;">${cleaned}</code>
                <button type="button" class="merge-svc-remove" data-code="${cleaned}"
                    style="background:none;border:none;cursor:pointer;color:#b45309;font-size:0.85rem;padding:0;line-height:1;"
                    title="Remove ${cleaned}">×</button>`;
            // Insert chip before the text input
            const input = tagsContainer.querySelector('#merge-svc-input');
            tagsContainer.insertBefore(chip, input);
            // Wire up its remove button
            chip.querySelector('.merge-svc-remove').addEventListener('click', () => {
                chip.remove();
                saveMergeConfig();
            });
            saveMergeConfig();
        };

        // Mode radio buttons
        form.querySelectorAll('input[name="collapse_text_obx_mode"]').forEach(radio => {
            radio.addEventListener('change', () => {
                const sec = form.querySelector('#collapse-services-section');
                if (sec) sec.style.display = radio.value === 'by_service' ? 'block' : 'none';
                saveMergeConfig();
            });
        });

        // Remove buttons on pre-existing chips
        form.querySelectorAll('.merge-svc-remove').forEach(btn => {
            btn.addEventListener('click', () => {
                btn.closest('.merge-svc-chip')?.remove();
                saveMergeConfig();
            });
        });

        // Tag text input — Enter or comma adds a chip
        const svcInput = form.querySelector('#merge-svc-input');
        if (svcInput) {
            svcInput.addEventListener('keydown', (e) => {
                if (e.key === 'Enter' || e.key === ',') {
                    e.preventDefault();
                    const codes = svcInput.value.split(/[,\s]+/).filter(Boolean);
                    codes.forEach(addServiceTag);
                    svcInput.value = '';
                } else if (e.key === 'Backspace' && svcInput.value === '') {
                    // Remove last chip on backspace when input is empty
                    const chips = form.querySelectorAll('.merge-svc-chip');
                    if (chips.length > 0) {
                        chips[chips.length - 1].remove();
                        saveMergeConfig();
                    }
                }
            });
            svcInput.addEventListener('blur', () => {
                const codes = svcInput.value.split(/[,\s]+/).filter(Boolean);
                if (codes.length > 0) {
                    codes.forEach(addServiceTag);
                    svcInput.value = '';
                }
            });
            // Make clicking anywhere in the tags container focus the input
            const tagsContainer = form.querySelector('#merge-svc-tags');
            if (tagsContainer) {
                tagsContainer.addEventListener('click', (e) => {
                    if (!e.target.closest('button')) svcInput.focus();
                });
            }
        }

        // Quick-add hint buttons
        form.querySelectorAll('.merge-svc-hint').forEach(btn => {
            btn.addEventListener('click', () => addServiceTag(btn.dataset.code));
        });

        // === Mapping Search ===
        const searchInput = form.querySelector('#mappingSearchInput');
        if (searchInput) {
            searchInput.addEventListener('input', (e) => {
                this.filterMappingTable(e.target.value);
            });
        }


        // === Upload File ===
        const uploadInput = form.querySelector('#uploadMappingFile');
        if (uploadInput) {
            uploadInput.addEventListener('change', (e) => {
                this.handleMappingFileUpload(e.target.files[0], step);
            });
        }

        // === Drag & Drop Upload ===
        const uploadDropZone = form.querySelector('#uploadDropZone');
        if (uploadDropZone) {
            uploadDropZone.addEventListener('dragover', (e) => {
                e.preventDefault();
                uploadDropZone.style.borderColor = '#1e3a8a';
                uploadDropZone.style.background = '#f0f9ff';
            });
            uploadDropZone.addEventListener('dragleave', () => {
                uploadDropZone.style.borderColor = '#cbd5e1';
                uploadDropZone.style.background = '#f8fafc';
            });
            uploadDropZone.addEventListener('drop', (e) => {
                e.preventDefault();
                uploadDropZone.style.borderColor = '#cbd5e1';
                uploadDropZone.style.background = '#f8fafc';
                const file = e.dataTransfer.files[0];
                if (file && file.type === 'application/json') {
                    this.handleMappingFileUpload(file, step);
                }
            });
        }

        // === Export Mappings ===
        const exportBtn = form.querySelector('#exportMappingsBtn');
        if (exportBtn) {
            exportBtn.addEventListener('click', () => {
                this.exportMappingsAsJSON(step);
            });
        }

        // === Add Mapping Button (NO-CODE) ===
        const addMappingBtn = form.querySelector('#addMappingBtn');
        if (addMappingBtn) {
            addMappingBtn.addEventListener('click', () => {
                // Initialize mappings array if it doesn't exist
                if (!step.config) {
                    step.config = {};
                }
                if (!step.config.mappings) {
                    step.config.mappings = [];
                }

                // Show edit modal in "add" mode (index undefined)
                this.editMapping(undefined);
            });
        }

        // === Load Standard Template Button ===
        const loadTemplateBtn = form.querySelector('#loadStandardTemplateBtn');
        if (loadTemplateBtn) {
            loadTemplateBtn.addEventListener('click', async () => {
                await this.loadStandardTemplateMappings(step);
            });
        }

        // === Auto-load standard template if needed ===
        const mappingTableContainer = form.querySelector('#mappingTableContainer');
        if (mappingTableContainer && mappingTableContainer.dataset.autoLoadTemplate === 'true') {
            console.log('🔄 Auto-loading standard template mappings...');
            // Auto-fetch template mappings asynchronously
            this.autoLoadTemplateMappings(step, mappingTableContainer);
        }

        // === Resources Tab: Load Preview Button ===
        const loadResourcePreviewBtn = form.querySelector('#loadResourcePreviewBtn');
        if (loadResourcePreviewBtn) {
            loadResourcePreviewBtn.addEventListener('click', () => {
                this.loadResourcePreview(step, form);
            });
            // Auto-load if switching to resources tab
            const resourcesTab = form.querySelector('[data-tab="resources"]');
            if (resourcesTab) {
                resourcesTab.addEventListener('click', () => {
                    const cardsArea = form.querySelector('#resourceCardsArea');
                    if (cardsArea && cardsArea.querySelector('.resource-card') === null) {
                        this.loadResourcePreview(step, form);
                    }
                });
            }
        }

        // === JSON Format Button ===
        const formatJsonBtn = form.querySelector('#formatJsonBtn');
        if (formatJsonBtn) {
            formatJsonBtn.addEventListener('click', () => {
                const textarea = form.querySelector('#stepConfig');
                try {
                    const obj = JSON.parse(textarea.value);
                    textarea.value = JSON.stringify(obj, null, 2);
                    this.builder.dragDropManager.showNotification('JSON formatted', 'success');
                } catch (error) {
                    this.builder.dragDropManager.showNotification('Invalid JSON', 'error');
                }
            });
        }

        // === JSON Validate Button ===
        const validateJsonBtn = form.querySelector('#validateJsonBtn');
        if (validateJsonBtn) {
            validateJsonBtn.addEventListener('click', () => {
                const textarea = form.querySelector('#stepConfig');
                this.validateJSON(textarea);
            });
        }

        // === Validation Builder Initialization (Modular OOP Component) ===
        const builderContainers = form.querySelectorAll('.validation-builder-container');
        builderContainers.forEach(container => {
            const initialRulesJSON = container.dataset.initialRules;
            const initialRules = initialRulesJSON ? JSON.parse(initialRulesJSON) : [];

            // Get message type from pipeline builder context
            const messageType = this.builder.messageType || 'ADT_A01';

            // Instantiate ValidationRuleBuilder component with schema context
            const builder = new ValidationRuleBuilder(container, initialRules, {
                format: 'hl7v2',
                version: 'v2.5',  // TODO: Get from interface configuration
                messageType: messageType
            });

            // Store reference for later access
            container._validationBuilderInstance = builder;
        });

        // === Metadata Builder Initialization (Key-Value Pair Builder) ===
        const metadataContainers = form.querySelectorAll('.metadata-builder-container');
        metadataContainers.forEach(container => {
            const initialMetadataJSON = container.dataset.initialMetadata;
            const initialMetadata = initialMetadataJSON ? JSON.parse(initialMetadataJSON) : {};

            // Instantiate MetadataBuilder component
            const builder = new MetadataBuilder(container, initialMetadata);

            // Store reference for later access
            container._metadataBuilderInstance = builder;
        });

        // === Header Builder Initialization (HTTP Headers for API Enrichment) ===
        const headerContainers = form.querySelectorAll('.header-builder-container');
        headerContainers.forEach(container => {
            const initialHeadersJSON = container.dataset.initialHeaders;
            const initialHeaders = initialHeadersJSON ? JSON.parse(initialHeadersJSON) : {};

            // Instantiate HeaderBuilder component
            const builder = new HeaderBuilder(container, initialHeaders);

            // Store reference for later access
            container._headerBuilderInstance = builder;
        });

        // === Query Param Builder Initialization (Query Parameters for API Enrichment & Database Enrichment) ===
        const queryParamContainers = form.querySelectorAll('.query-param-builder-container');
        console.log('[PropertiesPanel] 🔍 Found', queryParamContainers.length, 'query-param-builder-container(s) during initialization');
        queryParamContainers.forEach((container, index) => {
            const initialParamsJSON = container.dataset.initialParams;
            const initialParams = initialParamsJSON ? JSON.parse(initialParamsJSON) : {};
            const fieldKey = container.dataset.fieldKey;

            console.log(`[PropertiesPanel] 📦 Initializing QueryParamBuilder #${index} with fieldKey="${fieldKey}", initialParams:`, initialParams);

            // Instantiate QueryParamBuilder component
            const builder = new QueryParamBuilder(container, initialParams);

            // Store reference for later access (both on container AND on this object)
            container._queryParamBuilderInstance = builder;

            // CRITICAL FIX: Also store globally on PropertiesPanel instance
            // This ensures we can retrieve it even if the modal content is replaced
            this._activeQueryParamBuilder = builder;
            this._activeQueryParamFieldKey = fieldKey;

            // Listen for parameter changes to update the Database Query Tester in real-time
            container.addEventListener('paramsChanged', (e) => {
                console.log('[PropertiesPanel] 📡 Query params changed:', e.detail.params);
                // Find and update the database query tester
                const testerContainers = form.querySelectorAll('.database-query-tester-container');
                testerContainers.forEach(testerContainer => {
                    const tester = testerContainer._databaseTesterInstance;
                    if (tester) {
                        const currentConfig = tester.config || {};
                        currentConfig.queryParams = e.detail.params;
                        tester.updateConfig(currentConfig);
                        console.log('[PropertiesPanel] ✅ Updated tester with new params');
                    }
                });
            });

            console.log(`[PropertiesPanel] ✅ Stored _queryParamBuilderInstance for fieldKey="${fieldKey}"`);
        });

        // === Result Mapping Builder Initialization (Database Enrichment) ===
        // NO-CODE: Visual builder for mapping database columns to output fields
        const resultMappingContainers = form.querySelectorAll('.result-mapping-builder-container');
        resultMappingContainers.forEach(container => {
            const initialMappingsJSON = container.dataset.initialMappings;
            const initialMappings = initialMappingsJSON ? JSON.parse(initialMappingsJSON) : {};

            // Instantiate ResultMappingBuilder component
            const builder = new ResultMappingBuilder(container, initialMappings);

            // Store reference for later access
            container._resultMappingBuilderInstance = builder;
        });

        // === API Response Mapping Builder Initialization (API Enrichment) ===
        // NO-CODE: Visual builder for mapping API response fields to output variables
        const apiResponseMappingContainers = form.querySelectorAll('.api-response-mapping-builder-container');
        apiResponseMappingContainers.forEach(container => {
            const initialMappingsJSON = container.dataset.initialMappings;
            let initialMappings = { extractors: [] };
            try {
                initialMappings = initialMappingsJSON ? JSON.parse(initialMappingsJSON) : { extractors: [] };
            } catch (e) {
                console.warn('[PropertiesPanel] Failed to parse initial API response mappings:', e);
            }

            // Render the API Response Mapping Builder UI
            this.renderApiResponseMappingBuilder(container, initialMappings, step);
        });

        // === MongoDB Filter Builder Initialization ===
        // NO-CODE: Visual builder for MongoDB filter queries
        const mongoFilterContainers = form.querySelectorAll('.mongodb-filter-builder-container');
        mongoFilterContainers.forEach(container => {
            const initialFilterJSON = container.dataset.initialFilter;
            const initialFilter = initialFilterJSON ? JSON.parse(initialFilterJSON) : {};

            // Instantiate MongoDBFilterBuilder component
            const builder = new MongoDBFilterBuilder(container, initialFilter);

            // Store reference for later access
            container._mongodbFilterBuilderInstance = builder;
        });

        // === MongoDB Projection Builder Initialization ===
        // NO-CODE: Visual field selector for MongoDB projection
        const mongoProjectionContainers = form.querySelectorAll('.mongodb-projection-builder-container');
        mongoProjectionContainers.forEach(container => {
            const initialProjectionJSON = container.dataset.initialProjection;
            const initialProjection = initialProjectionJSON ? JSON.parse(initialProjectionJSON) : {};

            // Get connection config from form fields
            const connectionConfig = {
                dbHost: form.querySelector('[name="config_dbHost"]')?.value || 'mongodb',
                dbPort: parseInt(form.querySelector('[name="config_dbPort"]')?.value) || 27017,
                dbName: form.querySelector('[name="config_dbName"]')?.value || 'ezhealthkonnect',
                dbUser: form.querySelector('[name="config_dbUser"]')?.value || '',
                dbPassword: form.querySelector('[name="config_dbPassword"]')?.value || '',
                collection: form.querySelector('[name="config_collection"]')?.value || ''
            };

            // Instantiate MongoDBProjectionBuilder component with connection config
            const builder = new MongoDBProjectionBuilder(container, initialProjection, connectionConfig);

            // Store reference for later access
            container._mongodbProjectionBuilderInstance = builder;

            // Watch for collection name changes and reload schema
            const collectionInput = form.querySelector('[name="config_collection"]');
            if (collectionInput) {
                collectionInput.addEventListener('change', () => {
                    connectionConfig.collection = collectionInput.value;
                    builder.connectionConfig = connectionConfig;
                    builder.loadCollectionSchema();
                });
            }
        });

        // === Redis Query Builder Initialization ===
        // NO-CODE: Visual builder for Redis queries
        const redisQueryBuilderContainers = form.querySelectorAll('.redis-query-builder-container');
        redisQueryBuilderContainers.forEach(container => {
            // Get all Redis config from dataset
            const redisConfig = {
                redisQuery: container.dataset.redisQuery || '',
                redisCommand: container.dataset.redisCommand || '',
                redisKey: container.dataset.redisKey || '',
                redisHashField: container.dataset.redisHashField || ''
            };

            // Instantiate RedisQueryBuilder component
            const builder = new RedisQueryBuilder(container, redisConfig);

            // Store reference for later access
            container._redisQueryBuilderInstance = builder;
        });

        // === Database Query Tester Initialization (Database Enrichment) ===
        // NO-CODE: Test SQL queries before saving pipeline
        const dbQueryTesterContainers = form.querySelectorAll('.database-query-tester-container');
        dbQueryTesterContainers.forEach(container => {
            // Create tester with empty config (will be updated when user changes fields)
            const tester = new DatabaseQueryTester(container, {});

            // Store reference for later access
            container._databaseQueryTesterInstance = tester;

            // Set callback for adding mappings from query results
            tester.setOnAddMapping((dbColumn) => {
                console.log('🎯 User clicked Add to Mapping for column:', dbColumn);

                // Find the ResultMappingBuilder instance
                const resultMappingContainer = form.querySelector('.result-mapping-builder-container');
                if (resultMappingContainer && resultMappingContainer._resultMappingBuilderInstance) {
                    resultMappingContainer._resultMappingBuilderInstance.addMappingFromQueryResult(dbColumn);
                    console.log('✅ Added column to Result Mapping Builder:', dbColumn);
                } else {
                    console.warn('⚠️ ResultMappingBuilder not found');
                }
            });

            // Update tester config when user changes query/params/connection
            const updateTesterConfig = () => {
                const config = {
                    databaseType: form.querySelector('[name="config_databaseType"]')?.value,
                    connectionString: form.querySelector('[name="config_connectionString"]')?.value,
                    // Individual connection fields
                    dbHost: form.querySelector('[name="config_dbHost"]')?.value,
                    dbPort: parseInt(form.querySelector('[name="config_dbPort"]')?.value) || 0,
                    dbName: form.querySelector('[name="config_dbName"]')?.value,
                    dbUser: form.querySelector('[name="config_dbUser"]')?.value,
                    dbPassword: form.querySelector('[name="config_dbPassword"]')?.value,
                    query: form.querySelector('[name="config_query"]')?.value,
                    queryParams: {}
                };

                // Get query params from QueryParamBuilder
                const queryParamContainer = form.querySelector('.query-param-builder-container');
                if (queryParamContainer && queryParamContainer._queryParamBuilderInstance) {
                    config.queryParams = queryParamContainer._queryParamBuilderInstance.getParams();
                } else if (this._activeQueryParamBuilder) {
                    // FALLBACK: Use globally stored instance if container not found
                    config.queryParams = this._activeQueryParamBuilder.getParams();
                    console.log('[PropertiesPanel] 🔧 Using global QueryParamBuilder for tester update:', config.queryParams);
                }

                tester.updateConfig(config);
            };

            // Attach change listeners to update tester config
            const queryInput = form.querySelector('[name="config_query"]');
            if (queryInput) {
                queryInput.addEventListener('blur', updateTesterConfig);
            }

            const connectionStringInput = form.querySelector('[name="config_connectionString"]');
            if (connectionStringInput) {
                connectionStringInput.addEventListener('blur', updateTesterConfig);
            }

            const databaseTypeSelect = form.querySelector('[name="config_databaseType"]');
            if (databaseTypeSelect) {
                databaseTypeSelect.addEventListener('change', updateTesterConfig);
            }

            // Attach listeners for individual connection fields
            const dbFieldNames = ['config_dbHost', 'config_dbPort', 'config_dbName', 'config_dbUser', 'config_dbPassword'];
            dbFieldNames.forEach(fieldName => {
                const field = form.querySelector(`[name="${fieldName}"]`);
                if (field) {
                    field.addEventListener('blur', updateTesterConfig);
                }
            });

            // Initial config update
            setTimeout(updateTesterConfig, 100);
        });

        // === OAuth 2.0 Config Builder Initialization (OAuth 2.0 for API Enrichment) ===
        const oauth2Containers = form.querySelectorAll('.oauth2-config-builder-container');
        oauth2Containers.forEach(container => {
            const initialConfigJSON = container.dataset.initialConfig;
            const initialConfig = initialConfigJSON ? JSON.parse(initialConfigJSON) : {};

            // Instantiate OAuth2ConfigBuilder component
            const builder = new OAuth2ConfigBuilder(container, initialConfig);

            // Store reference for later access
            container._oauth2ConfigBuilderInstance = builder;
        });

        // === API Endpoint Tester Initialization (Test API Before Configuring Mapping) ===
        // NO-CODE INTEGRATION ENGINE: Let users see API response before configuration
        const apiTesterContainers = form.querySelectorAll('.api-endpoint-tester-container');
        console.log('🔍 Found API tester containers:', apiTesterContainers.length);
        apiTesterContainers.forEach(container => {
            console.log('🔍 Container element:', container);
            console.log('🔍 Container ID:', container.id);

            if (typeof APIEndpointTester === 'undefined') {
                console.warn('APIEndpointTester component not loaded');
                return;
            }

            // Instantiate API Endpoint Tester component - pass element directly
            const tester = new APIEndpointTester(container);

            // Set callback for when user clicks a field to add to mapping
            tester.setOnAddMappingRule((ruleData) => {
                console.log('🎯 User added field to response mapping:', ruleData);

                // Get or create response mapping config
                if (!step.config.responseMapping) {
                    step.config.responseMapping = {
                        mode: 'custom',
                        extractors: []
                    };
                }

                // Add the new extractor rule
                step.config.responseMapping.extractors.push({
                    sourcePath: ruleData.sourcePath,
                    targetField: ruleData.targetField,
                    transformType: ruleData.transformType || 'none',
                    required: false,
                    description: `Extracted from ${ruleData.sourcePath}`
                });

                // Show success feedback
                console.log('✅ Added field to response mapping:', ruleData.targetField);

                // TODO: Re-render response mapping section to show new rule in UI
                // For now, user can see it when they save and reopen the step
            });

            // Get current step configuration for testing
            const getCurrentStepConfig = () => {
                const config = {};

                // Get endpoint
                const endpointInput = form.querySelector('[name="config_endpoint"]');
                if (endpointInput) config.endpoint = endpointInput.value;

                // Get method
                const methodSelect = form.querySelector('[name="config_method"]');
                if (methodSelect) config.method = methodSelect.value;

                // Get auth type
                const authTypeSelect = form.querySelector('[name="config_authType"]');
                if (authTypeSelect) config.authType = authTypeSelect.value;

                // Get bearer token if present
                const bearerTokenInput = form.querySelector('[name="config_bearerToken"]');
                if (bearerTokenInput) config.bearerToken = bearerTokenInput.value;

                // Get API key if present
                const apiKeyInput = form.querySelector('[name="config_apiKey"]');
                if (apiKeyInput) config.apiKey = apiKeyInput.value;

                // Get OAuth2 config from OAuth2ConfigBuilder component
                const oauth2Container = form.querySelector('.oauth2-config-builder');
                if (oauth2Container && oauth2Container._oauth2ConfigBuilderInstance) {
                    const oauth2Config = oauth2Container._oauth2ConfigBuilderInstance.getConfig();
                    console.log('[PropertiesPanel] 🔍 Reading OAuth2 config for API test:', oauth2Config);

                    // Map OAuth2ConfigBuilder fields to backend model fields
                    if (oauth2Config.tokenURL) config.oauth2TokenUrl = oauth2Config.tokenURL;
                    if (oauth2Config.clientID) config.oauth2ClientId = oauth2Config.clientID;
                    if (oauth2Config.clientSecret) config.oauth2ClientSecret = oauth2Config.clientSecret;
                    if (oauth2Config.grantType) config.oauth2GrantType = oauth2Config.grantType;
                    if (oauth2Config.scope) config.oauth2Scope = oauth2Config.scope;
                    if (oauth2Config.audience) config.oauth2Audience = oauth2Config.audience;
                    if (oauth2Config.username) config.oauth2Username = oauth2Config.username;
                    if (oauth2Config.password) config.oauth2Password = oauth2Config.password;
                }

                // Get headers from builder
                const headerContainer = form.querySelector('.header-builder-container');
                if (headerContainer && headerContainer._headerBuilderInstance) {
                    config.headers = headerContainer._headerBuilderInstance.getHeaders();
                }

                // Get query params from builder
                const queryParamContainer = form.querySelector('.query-param-builder-container');
                if (queryParamContainer && queryParamContainer._queryParamBuilderInstance) {
                    config.queryParams = queryParamContainer._queryParamBuilderInstance.getQueryParams();
                }

                // Get field mappings (for URL placeholder resolution)
                const fieldMappingsInput = form.querySelector('[name="config_fieldMappings"]');
                if (fieldMappingsInput && fieldMappingsInput.value) {
                    try {
                        config.fieldMappings = JSON.parse(fieldMappingsInput.value);
                    } catch (e) {
                        console.warn('Failed to parse fieldMappings:', e);
                    }
                }

                return config;
            };

            // Render the tester - pass the FUNCTION so it gets fresh config on each test
            tester.render(getCurrentStepConfig);

            // Store reference for later access
            container._apiEndpointTesterInstance = tester;
        });

        // === Field Path Selector Initialization (Universal - All Steps) ===
        // Auto-enhance all inputs with data-field-type="xpath"
        if (window.FieldPathSelector) {
            const messageType = this.builder.messageType || 'ADT_A01';

            window.FieldPathSelector.initialize(form, {
                format: 'hl7v2',
                version: 'v2.5',
                messageType: messageType
            });
        }

        // === Generic Field Mapping Button (for core.transformation steps) ===
        const addGenericMappingBtn = form.querySelector('#addGenericMappingBtn');
        if (addGenericMappingBtn) {
            addGenericMappingBtn.addEventListener('click', () => {
                // Initialize mappings array if needed
                if (!step.config) step.config = {};
                if (!step.config.mappings) step.config.mappings = [];

                // Call inline save method
                this.saveGenericMapping();
            });
        }

        // Initialize smart search for RHS field
        const newMappingRHS = form.querySelector('#newMappingRHS');
        if (newMappingRHS && typeof FieldPathSearchComponent !== 'undefined') {
            new FieldPathSearchComponent(newMappingRHS, {
                placeholder: 'Search HL7 fields or enter variable...',
                allowCustom: true,
                showCategories: true,
                onSelect: (fieldPath) => {
                    newMappingRHS.value = fieldPath;
                }
            });
        }

        // Allow Enter key to add mapping
        const newMappingLHS = form.querySelector('#newMappingLHS');
        const newMappingTransforms = form.querySelector('#newMappingTransforms');
        [newMappingLHS, newMappingRHS].forEach(input => {
            if (input) {
                input.addEventListener('keypress', (e) => {
                    if (e.key === 'Enter') {
                        e.preventDefault();
                        addGenericMappingBtn.click();
                    }
                });
            }
        });

        // === Error Handling & Retry Toggle Events ===
        const ehSection = form.querySelector('.error-handling-section');
        if (ehSection) {
            const ehCheckbox = ehSection.querySelector('#ehEnabled');
            if (ehCheckbox) {
                ehCheckbox.addEventListener('change', () => {
                    const on = ehCheckbox.checked;
                    const track = ehCheckbox.nextElementSibling;
                    const knob = track?.nextElementSibling;
                    if (track) track.style.background = on ? '#ef4444' : '#cbd5e1';
                    if (knob) knob.style.left = on ? '18px' : '2px';
                    const configArea = ehSection.querySelector('#ehConfigArea');
                    if (configArea) {
                        configArea.style.opacity = on ? '1' : '0.4';
                        configArea.style.pointerEvents = on ? 'auto' : 'none';
                    }
                });
            }

            const retryCheckbox = ehSection.querySelector('#retryEnabled');
            if (retryCheckbox) {
                retryCheckbox.addEventListener('change', () => {
                    const on = retryCheckbox.checked;
                    const track = retryCheckbox.nextElementSibling;
                    const knob = track?.nextElementSibling;
                    if (track) track.style.background = on ? '#8b5cf6' : '#cbd5e1';
                    if (knob) knob.style.left = on ? '18px' : '2px';
                    const configArea = ehSection.querySelector('#retryConfigArea');
                    if (configArea) {
                        configArea.style.opacity = on ? '1' : '0.4';
                        configArea.style.pointerEvents = on ? 'auto' : 'none';
                    }
                });
            }
        }

        // === Conditional Field Visibility (Dynamic Auth Form Fields) ===
        this.setupConditionalFieldVisibility(form);

        return form;
    }

    /**
     * Setup conditional field visibility based on control field values
     * Used for dynamic auth form fields and other conditional configurations
     */
    setupConditionalFieldVisibility(form) {
        // Find all fields that control visibility of other fields
        const controlFields = form.querySelectorAll('select[name^="config_"], input[name^="config_"]');

        controlFields.forEach(controlField => {
            const fieldName = controlField.name.replace('config_', '');

            // Find all conditional fields that depend on this control field (old pattern)
            const conditionalFields = form.querySelectorAll(
                `.conditional-field[data-visible-when-field="${fieldName}"]`
            );

            // Find all conditional fields that depend on this control field (new showIf pattern)
            const showIfFields = form.querySelectorAll(
                `.conditional-field[data-show-if-field="${fieldName}"]`
            );

            if (conditionalFields.length === 0 && showIfFields.length === 0) return;

            // Add change event listener to control field
            controlField.addEventListener('change', (e) => {
                const currentValue = e.target.value;

                // Update visibility of all dependent fields (old pattern)
                conditionalFields.forEach(conditionalField => {
                    this.updateFieldVisibility(conditionalField, form);
                });

                // Update visibility of all dependent fields (new showIf pattern with arrays)
                showIfFields.forEach(conditionalField => {
                    this.updateFieldVisibility(conditionalField, form);
                });
            });

            // Trigger initial visibility check
            controlField.dispatchEvent(new Event('change'));
        });
    }

    /**
     * Update field visibility based on ALL conditions (AND logic)
     * Checks both visibleWhen and showIf conditions if present
     */
    updateFieldVisibility(conditionalField, form) {
        let shouldShow = true;

        // Check visibleWhen condition
        const visibleWhenField = conditionalField.dataset.visibleWhenField;
        const visibleWhenValue = conditionalField.dataset.visibleWhenValue;
        if (visibleWhenField && visibleWhenValue) {
            const controlField = form.querySelector(`[name="config_${visibleWhenField}"]`);
            if (controlField && controlField.value !== visibleWhenValue) {
                shouldShow = false;
            }
        }

        // Check showIf condition (AND with visibleWhen)
        const showIfField = conditionalField.dataset.showIfField;
        const showIfValues = conditionalField.dataset.showIfValues;
        if (showIfField && showIfValues) {
            const controlField = form.querySelector(`[name="config_${showIfField}"]`);
            const allowedValues = JSON.parse(showIfValues || '[]');
            if (controlField && !allowedValues.includes(controlField.value)) {
                shouldShow = false;
            }
        }

        // Apply visibility
        if (shouldShow) {
            conditionalField.classList.remove('hidden');
        } else {
            conditionalField.classList.add('hidden');
        }
    }

    /**
     * Attach JSON import/export event listeners
     */
    attachJSONImportEvents(container, step, isPreview = false) {
        // Import JSON button
        const importBtn = container.querySelector('#importJsonBtn');
        if (importBtn) {
            importBtn.addEventListener('click', () => {
                const textarea = container.querySelector('#jsonConfigInput');
                try {
                    const importedConfig = JSON.parse(textarea.value);

                    // Merge imported config into step
                    Object.assign(step, importedConfig);

                    if (isPreview) {
                        // Add to pipeline with imported config
                        this.addStepToPipeline(step);
                    } else {
                        // Update existing step
                        this.saveStepProperties(step);
                    }
                } catch (error) {
                    this.builder.dragDropManager.showNotification(
                        `Invalid JSON: ${error.message}`,
                        'error'
                    );
                }
            });
        }

        // Export JSON button
        const exportBtn = container.querySelector('#exportJsonBtn');
        if (exportBtn) {
            exportBtn.addEventListener('click', () => {
                const textarea = container.querySelector('#jsonConfigInput');
                const config = {
                    stepName: step.stepName,
                    stepType: step.stepType,
                    sequence: step.sequence,
                    enabled: step.enabled,
                    config: step.config || {},
                    scriptContent: step.scriptContent || ''
                };

                // Download as JSON file
                const blob = new Blob([JSON.stringify(config, null, 2)], { type: 'application/json' });
                const url = URL.createObjectURL(blob);
                const a = document.createElement('a');
                a.href = url;
                a.download = `${step.stepName.replace(/\s+/g, '_')}_config.json`;
                a.click();
                URL.revokeObjectURL(url);

                this.builder.dragDropManager.showNotification('Configuration exported', 'success');
            });
        }

        // Cancel button
        const cancelBtn = container.querySelector('#cancelStepBtn');
        if (cancelBtn) {
            cancelBtn.addEventListener('click', () => this.closeModal());
        }

        // Format JSON button
        const formatBtn = container.querySelector('#formatJsonBtn');
        if (formatBtn) {
            formatBtn.addEventListener('click', () => {
                const textarea = container.querySelector('#jsonConfigInput');
                try {
                    const obj = JSON.parse(textarea.value);
                    textarea.value = JSON.stringify(obj, null, 2);
                    this.showValidationStatus(container, true, 'JSON formatted successfully');
                } catch (error) {
                    this.showValidationStatus(container, false, `Invalid JSON: ${error.message}`);
                }
            });
        }

        // Validate JSON button
        const validateBtn = container.querySelector('#validateJsonBtn');
        if (validateBtn) {
            validateBtn.addEventListener('click', () => {
                const textarea = container.querySelector('#jsonConfigInput');
                try {
                    JSON.parse(textarea.value);
                    this.showValidationStatus(container, true, 'Valid JSON');
                } catch (error) {
                    this.showValidationStatus(container, false, `Invalid JSON: ${error.message}`);
                }
            });
        }

        // Copy JSON button
        const copyBtn = container.querySelector('#copyJsonBtn');
        if (copyBtn) {
            copyBtn.addEventListener('click', async () => {
                const textarea = container.querySelector('#jsonConfigInput');
                try {
                    await navigator.clipboard.writeText(textarea.value);
                    this.showValidationStatus(container, true, 'Copied to clipboard');
                } catch (error) {
                    this.builder.dragDropManager.showNotification('Failed to copy', 'error');
                }
            });
        }

        // Clear JSON button
        const clearBtn = container.querySelector('#clearJsonBtn');
        if (clearBtn) {
            clearBtn.addEventListener('click', async () => {
                const textarea = container.querySelector('#jsonConfigInput');
                const confirmed = await this.builder.dragDropManager.showConfirmDialog(
                    'Are you sure you want to clear the JSON configuration?',
                    {
                        title: 'Clear JSON',
                        confirmText: 'Clear',
                        cancelText: 'Cancel',
                        type: 'warning'
                    }
                );
                if (confirmed) {
                    textarea.value = '';
                    this.showValidationStatus(container, true, 'JSON cleared');
                }
            });
        }
    }

    /**
     * Show validation status message
     */
    showValidationStatus(container, isValid, message) {
        const statusDiv = container.querySelector('#jsonValidationStatus');
        if (!statusDiv) return;

        statusDiv.style.display = 'flex';
        statusDiv.className = isValid ? 'json-validation-status json-valid' : 'json-validation-status json-invalid';
        statusDiv.innerHTML = `
            <i class="fas fa-${isValid ? 'check-circle' : 'exclamation-triangle'}"></i>
            <span>${message}</span>
        `;

        setTimeout(() => {
            statusDiv.style.display = 'none';
        }, 3000);
    }

    /**
     * Filter mapping table by search query
     */
    filterMappingTable(query) {
        const rows = this.container.querySelectorAll('.mapping-row');
        const lowerQuery = query.toLowerCase();

        rows.forEach(row => {
            const hl7Field = row.querySelector('code')?.textContent.toLowerCase() || '';
            const fhirPath = row.querySelectorAll('code')[1]?.textContent.toLowerCase() || '';

            if (hl7Field.includes(lowerQuery) || fhirPath.includes(lowerQuery)) {
                row.style.display = '';
            } else {
                row.style.display = 'none';
            }
        });
    }

    /**
     * Create generic field mapping configuration section (NO-CODE for generic transformations)
     * Color theme: White background, Navy blue (#1e3a8a) primary, Pastel pink (#ffc0cb) accents
     */
    createGenericFieldMappingSection(step) {
        // Initialize mappings array if it doesn't exist
        if (!step.config) {
            step.config = {};
        }
        if (!step.config.mappings) {
            step.config.mappings = [];
        }

        const mappings = step.config.mappings || [];

        return `
            <div class="form-section">
                <h4 style="color: #1e3a8a; margin-bottom: 1rem; font-weight: 600;">Variable Assignments</h4>

                <!-- Subtle Info Box with Navy Blue theme and Pastel Pink accent -->
                <div style="background: #f0f4ff; border-left: 4px solid #1e3a8a; border-right: 2px solid #ffc0cb; padding: 12px 16px; border-radius: 4px; margin-bottom: 1rem;">
                    <div style="color: #1e3a8a; font-weight: 600; font-size: 0.9rem; margin-bottom: 4px;">Create new variables from HL7 fields or existing variables</div>
                    <div style="color: #64748b; font-size: 0.85rem; line-height: 1.5;">
                        LHS = new variable name, RHS = source (HL7 field or existing variable). Apply transformations as needed.
                    </div>
                </div>

                <!-- System Variables Quick-Add -->
                <div style="background: white; border: 1px solid #e2e8f0; border-left: 3px solid #ffc0cb; border-radius: 6px; padding: 12px; margin-bottom: 1rem;">
                    <div style="color: #1e3a8a; font-weight: 600; font-size: 0.85rem; margin-bottom: 8px;">Quick Add System Variables:</div>
                    <div style="display: flex; gap: 8px; flex-wrap: wrap;">
                        <button type="button" onclick="window.propertiesPanel.addSystemVariable('timestamp')" style="background: white; border: 1px solid #cbd5e1; color: #1e3a8a; padding: 6px 12px; border-radius: 4px; cursor: pointer; font-size: 0.8rem; transition: all 0.2s;" onmouseover="this.style.background='#f0f4ff'; this.style.borderColor='#1e3a8a'" onmouseout="this.style.background='white'; this.style.borderColor='#cbd5e1'">
                            + Timestamp
                        </button>
                        <button type="button" onclick="window.propertiesPanel.addSystemVariable('correlationId')" style="background: white; border: 1px solid #cbd5e1; color: #1e3a8a; padding: 6px 12px; border-radius: 4px; cursor: pointer; font-size: 0.8rem; transition: all 0.2s;" onmouseover="this.style.background='#f0f4ff'; this.style.borderColor='#1e3a8a'" onmouseout="this.style.background='white'; this.style.borderColor='#cbd5e1'">
                            + Correlation ID (GUID)
                        </button>
                        <button type="button" onclick="window.propertiesPanel.addSystemVariable('messageId')" style="background: white; border: 1px solid #cbd5e1; color: #1e3a8a; padding: 6px 12px; border-radius: 4px; cursor: pointer; font-size: 0.8rem; transition: all 0.2s;" onmouseover="this.style.background='#f0f4ff'; this.style.borderColor='#1e3a8a'" onmouseout="this.style.background='white'; this.style.borderColor='#cbd5e1'">
                            + Message ID (UUID)
                        </button>
                        <button type="button" onclick="window.propertiesPanel.addSystemVariable('interfaceId')" style="background: white; border: 1px solid #cbd5e1; color: #1e3a8a; padding: 6px 12px; border-radius: 4px; cursor: pointer; font-size: 0.8rem; transition: all 0.2s;" onmouseover="this.style.background='#f0f4ff'; this.style.borderColor='#1e3a8a'" onmouseout="this.style.background='white'; this.style.borderColor='#cbd5e1'">
                            + Interface ID
                        </button>
                        <button type="button" onclick="window.propertiesPanel.addSystemVariable('interfaceName')" style="background: white; border: 1px solid #cbd5e1; color: #1e3a8a; padding: 6px 12px; border-radius: 4px; cursor: pointer; font-size: 0.8rem; transition: all 0.2s;" onmouseover="this.style.background='#f0f4ff'; this.style.borderColor='#1e3a8a'" onmouseout="this.style.background='white'; this.style.borderColor='#cbd5e1'">
                            + Interface Name
                        </button>
                    </div>
                    <div style="margin-top: 6px; font-size: 0.7rem; color: #64748b;">
                        These system-generated values are automatically created at runtime
                    </div>
                </div>

                <!-- Inline Add Form -->
                <div style="background: white; border: 1px solid #e2e8f0; border-top: 2px solid #ffc0cb; border-radius: 6px; padding: 16px; margin-bottom: 1rem;">
                    <div style="display: grid; grid-template-columns: 1fr 1.5fr 1fr auto; gap: 12px; align-items: end;">
                        <div>
                            <label style="display: block; color: #64748b; font-size: 0.8rem; margin-bottom: 4px; font-weight: 500;">LHS (New Variable)</label>
                            <input type="text" id="newMappingLHS" placeholder="myVariable" style="width: 100%; padding: 8px; border: 1px solid #cbd5e1; border-radius: 4px; font-size: 13px; font-family: 'Courier New', monospace; transition: border-color 0.2s;" onfocus="this.style.borderColor='#ffc0cb'" onblur="this.style.borderColor='#cbd5e1'">
                        </div>
                        <div style="position: relative;">
                            <label style="display: block; color: #64748b; font-size: 0.8rem; margin-bottom: 4px; font-weight: 500;">RHS (Source - HL7 field or variable)</label>
                            <input type="text" id="newMappingRHS" placeholder="Search HL7 fields or enter variable..." style="width: 100%; padding: 8px; border: 1px solid #cbd5e1; border-radius: 4px; font-size: 13px; font-family: 'Courier New', monospace; transition: border-color 0.2s;" onfocus="this.style.borderColor='#ffc0cb'" onblur="this.style.borderColor='#cbd5e1'">
                        </div>
                        <div style="position: relative;">
                            <label style="display: block; color: #64748b; font-size: 0.8rem; margin-bottom: 4px; font-weight: 500;">
                                Transformations
                                <button type="button" onclick="window.propertiesPanel.showTransformHelp()" style="background: none; border: none; color: #ffc0cb; cursor: pointer; padding: 0; font-size: 0.75rem; margin-left: 4px;" title="View transformation guide">ℹ️</button>
                            </label>
                            <select id="newMappingTransforms" multiple onchange="window.propertiesPanel.handleTransformSelection()" style="width: 100%; padding: 4px; border: 1px solid #cbd5e1; border-radius: 4px; font-size: 12px; background: white; min-height: 38px; transition: border-color 0.2s;" onfocus="this.style.borderColor='#ffc0cb'" onblur="this.style.borderColor='#cbd5e1'">
                                <option value="trim">trim - Remove whitespace</option>
                                <option value="upper">upper - To UPPERCASE</option>
                                <option value="lower">lower - To lowercase</option>
                                <option value="regex">regex - Extract pattern</option>
                                <option value="substring">substring - Extract chars</option>
                                <option value="replace">replace - Replace text</option>
                            </select>
                            <div style="margin-top: 4px; display: none;" id="regexInput">
                                <input type="text" id="regexPattern" placeholder="Pattern: ^[0-9]+$" style="width: 100%; padding: 4px 8px; border: 1px solid #cbd5e1; border-radius: 3px; font-size: 11px; font-family: 'Courier New', monospace;">
                            </div>
                            <div style="margin-top: 4px; display: none;" id="substringInput">
                                <input type="text" id="substringParams" placeholder="start:end (e.g., 0:10)" style="width: 100%; padding: 4px 8px; border: 1px solid #cbd5e1; border-radius: 3px; font-size: 11px; font-family: 'Courier New', monospace;">
                            </div>
                            <div style="margin-top: 4px; display: none;" id="replaceInput">
                                <input type="text" id="replaceParams" placeholder="old:new (e.g., -:/)" style="width: 100%; padding: 4px 8px; border: 1px solid #cbd5e1; border-radius: 3px; font-size: 11px; font-family: 'Courier New', monospace;">
                            </div>
                        </div>
                        <button id="addGenericMappingBtn" style="background: #1e3a8a; color: white; border: none; padding: 8px 16px; border-radius: 4px; cursor: pointer; font-weight: 500; font-size: 0.9rem; white-space: nowrap; transition: background 0.2s;" onmouseover="this.style.background='#2563eb'" onmouseout="this.style.background='#1e3a8a'">
                            + Add
                        </button>
                    </div>
                    <div style="margin-top: 8px; font-size: 0.75rem; color: #64748b;">
                        RHS: Type to search HL7 fields (e.g., "PID", "Patient Name") or paste variable path | Use <strong>Ctrl/Cmd+Click</strong> to select multiple transformations |
                        <button type="button" id="addRegexBtn" onclick="window.propertiesPanel.addComplexTransform('regex')" style="background: none; border: none; color: #1e3a8a; cursor: pointer; padding: 0; font-size: 0.75rem;">+ Regex</button> |
                        <button type="button" id="addSubstringBtn" onclick="window.propertiesPanel.addComplexTransform('substring')" style="background: none; border: none; color: #1e3a8a; cursor: pointer; padding: 0; font-size: 0.75rem;">+ Substring</button> |
                        <button type="button" id="addReplaceBtn" onclick="window.propertiesPanel.addComplexTransform('replace')" style="background: none; border: none; color: #1e3a8a; cursor: pointer; padding: 0; font-size: 0.75rem;">+ Replace</button> |
                        <button type="button" onclick="window.propertiesPanel.showTransformHelp()" style="background: none; border: none; color: #1e3a8a; text-decoration: underline; cursor: pointer; padding: 0; font-size: 0.75rem;">Transform Guide</button>
                    </div>
                </div>

                <!-- Assignments List -->
                <div id="genericMappingsList" style="margin-top: 1rem;">
                    ${mappings.length > 0 ? `
                        <h5 style="color: #1e3a8a; margin-bottom: 0.5rem; font-weight: 600; font-size: 0.9rem;">Configured Variables (${mappings.length}):</h5>
                        <div style="background: white; border: 1px solid #e2e8f0; border-radius: 6px; overflow: hidden;">
                            ${mappings.map((mapping, index) => `
                                <div id="mapping-row-${index}" style="display: flex; align-items: center; gap: 12px; padding: 12px; border-bottom: 1px solid #e2e8f0; transition: background 0.15s;" onmouseover="this.style.background='#f8fafc'" onmouseout="this.style.background='white'">
                                    <div style="flex: 1;">
                                        <div style="font-size: 0.75rem; color: #64748b; margin-bottom: 2px;">Variable:</div>
                                        <input type="text"
                                               id="edit-lhs-${index}"
                                               value="${mapping.lhs}"
                                               onblur="window.propertiesPanel.updateMapping(${index}, 'lhs', this.value)"
                                               style="width: 100%; background: #f8fafc; padding: 4px 8px; border-radius: 3px; font-size: 12px; color: #334155; border: 1px solid #e2e8f0; font-family: 'Courier New', monospace;"
                                               onfocus="this.style.borderColor='#ffc0cb'; this.style.background='white'"
                                               onblur="this.style.borderColor='#e2e8f0'; this.style.background='#f8fafc'; window.propertiesPanel.updateMapping(${index}, 'lhs', this.value)">
                                    </div>
                                    <div style="flex: 0 0 40px; text-align: center; color: #64748b; font-size: 1.2rem;">=</div>
                                    <div style="flex: 1.5;">
                                        <div style="font-size: 0.75rem; color: #64748b; margin-bottom: 2px;">Source:</div>
                                        <textarea
                                               id="edit-rhs-${index}"
                                               onblur="window.propertiesPanel.updateMapping(${index}, 'rhs', this.value)"
                                               style="width: 100%; background: #f0f4ff; padding: 4px 8px; border-radius: 3px; font-size: 12px; color: #1e3a8a; border: 1px solid #dbeafe; font-family: 'Courier New', monospace; min-height: 36px; resize: vertical;"
                                               onfocus="this.style.borderColor='#ffc0cb'; this.style.background='white'"
                                               onblur="this.style.borderColor='#dbeafe'; this.style.background='#f0f4ff'; window.propertiesPanel.updateMapping(${index}, 'rhs', this.value)">${mapping.rhs}</textarea>
                                    </div>
                                    <div style="flex: 1;">
                                        <div style="font-size: 0.75rem; color: #64748b; margin-bottom: 2px;">Transforms:</div>
                                        <input type="text"
                                               id="edit-transforms-${index}"
                                               value="${mapping.transforms || ''}"
                                               placeholder="trim, upper, lower..."
                                               onblur="window.propertiesPanel.updateMapping(${index}, 'transforms', this.value)"
                                               style="width: 100%; background: #fdf2f8; color: #831843; padding: 4px 8px; border-radius: 3px; font-size: 11px; border: 1px solid #fce7f3; font-family: 'Courier New', monospace;"
                                               onfocus="this.style.borderColor='#ffc0cb'; this.style.background='white'"
                                               onblur="this.style.borderColor='#fce7f3'; this.style.background='#fdf2f8'; window.propertiesPanel.updateMapping(${index}, 'transforms', this.value)">
                                    </div>
                                    <button onclick="window.propertiesPanel.deleteGenericMapping(${index})" title="Delete" style="background: none; border: none; color: #94a3b8; cursor: pointer; transition: color 0.15s; font-size: 16px; padding: 8px;" onmouseover="this.style.color='#dc2626'" onmouseout="this.style.color='#94a3b8'">
                                        🗑️
                                    </button>
                                </div>
                            `).join('')}
                        </div>
                    ` : ''}
                </div>
            </div>
        `;
    }

    /**
     * Create If-Then-Else visual builder UI
     */
    createIfThenElseUI(step) {
        // Initialize config if it doesn't exist
        if (!step.config) {
            step.config = {
                conditions: [
                    {
                        name: 'Condition 1',
                        condition: {
                            field: '',
                            operator: 'equals',
                            value: '',
                            compareToField: ''
                        },
                        onTrue: {
                            action: 'continue'
                        },
                        onFalse: {
                            action: 'continue'
                        }
                    }
                ]
            };
        }

        const section = document.createElement('div');
        section.className = 'form-section';
        section.innerHTML = `
            <h4 style="color: var(--primary-color); margin-bottom: 1rem; display: flex; align-items: center; gap: 0.5rem;">
                <i class="fas fa-code-branch"></i>
                Conditional Logic Configuration
            </h4>
            <div id="if-then-else-builder-container" style="margin-top: 1rem;"></div>
        `;

        // Initialize builder after DOM insertion
        setTimeout(() => {
            const container = document.getElementById('if-then-else-builder-container');
            if (container && typeof IfThenElseBuilder !== 'undefined') {
                this.ifThenElseBuilder = new IfThenElseBuilder(container, step.config);
                console.log('✅ IfThenElseBuilder initialized with config:', step.config);
            } else {
                console.error('❌ IfThenElseBuilder not loaded or container not found');
                console.log('IfThenElseBuilder type:', typeof IfThenElseBuilder);
                console.log('Container:', container);
            }
        }, 0);

        return section.outerHTML;
    }

    /**
     * Create Switch/Case visual builder UI
     * Uses OOP pattern with BaseStepConfigBuilder.init()
     */
    createSwitchCaseUI(step) {
        // Initialize config if it doesn't exist (let builder handle defaults)
        if (!step.config) {
            step.config = {};
        }

        const section = document.createElement('div');
        section.className = 'form-section';
        section.innerHTML = `
            <div id="switch-case-builder-container" style="margin-top: 1rem;"></div>
        `;

        // Initialize builder after DOM insertion using OOP pattern
        setTimeout(() => {
            const container = document.getElementById('switch-case-builder-container');
            if (container && typeof SwitchCaseBuilder !== 'undefined') {
                // Create builder instance
                this.switchCaseBuilder = new SwitchCaseBuilder(container, step.config);

                // Call init() - Template Method Pattern from BaseStepConfigBuilder
                this.switchCaseBuilder.init();
                console.log('✅ SwitchCaseBuilder initialized (OOP pattern) with config:', step.config);

                // Listen for config changes from the builder
                container.addEventListener('configChange', (e) => {
                    step.config = e.detail.config;
                    console.log('📝 Switch/Case config updated:', step.config);
                });
            } else {
                console.error('❌ SwitchCaseBuilder not loaded or container not found');
                console.log('SwitchCaseBuilder type:', typeof SwitchCaseBuilder);
                console.log('Container:', container);
            }
        }, 0);

        return section.outerHTML;
    }

    /**
     * Create Loop Container visual builder UI
     * Uses OOP pattern with BaseStepConfigBuilder.init()
     */
    createLoopContainerUI(step) {
        // Initialize config if it doesn't exist (let builder handle defaults)
        if (!step.config) {
            step.config = {};
        }

        const section = document.createElement('div');
        section.className = 'form-section';
        section.innerHTML = `
            <div id="loop-container-builder-container" style="margin-top: 1rem;"></div>
        `;

        // Initialize builder after DOM insertion using OOP pattern
        setTimeout(() => {
            const container = document.getElementById('loop-container-builder-container');
            if (container && typeof ForEachLoopBuilder !== 'undefined') {
                // Create builder instance
                this.loopContainerBuilder = new ForEachLoopBuilder(container, step.config);

                // Call init() - Template Method Pattern from BaseStepConfigBuilder
                this.loopContainerBuilder.init();
                console.log('✅ ForEachLoopBuilder initialized (OOP pattern) with config:', step.config);

                // Listen for config changes from the builder
                container.addEventListener('configChange', (e) => {
                    step.config = e.detail.config;
                    console.log('📝 Loop config updated:', step.config);
                });
            } else {
                console.error('❌ ForEachLoopBuilder not loaded or container not found');
                console.log('ForEachLoopBuilder type:', typeof ForEachLoopBuilder);
                console.log('Container:', container);
            }
        }, 0);

        return section.outerHTML;
    }

    /**
     * Create Connector Configuration UI (Inbound/Outbound)
     * Uses ConnectorConfigBuilder OOP component
     */
    createConnectorConfigUI(step, direction) {
        if (!step.config) {
            step.config = {};
        }

        const section = document.createElement('div');
        section.className = 'form-section';
        section.innerHTML = `
            <div id="connector-config-builder-container" style="margin-top: 1rem;"></div>
        `;

        // Initialize builder after DOM insertion using OOP pattern
        setTimeout(() => {
            const container = document.getElementById('connector-config-builder-container');
            if (container && typeof ConnectorConfigBuilder !== 'undefined') {
                this.connectorConfigBuilder = new ConnectorConfigBuilder(container, step.config, direction, this);
                this.connectorConfigBuilder.init();
                console.log(`🔌 ConnectorConfigBuilder initialized (${direction}) with config:`, step.config);

                // Listen for config changes from the builder
                container.addEventListener('connectorConfigChanged', (e) => {
                    step.config = e.detail.config;
                    console.log('🔌 Connector config updated:', step.config);
                });
            } else {
                console.error('ConnectorConfigBuilder not loaded or container not found');
                if (container) {
                    container.innerHTML = `
                        <div class="alert alert-warning" style="margin: 12px 0;">
                            <i class="fas fa-exclamation-triangle"></i>
                            Connector configuration builder not available. Ensure ConnectorConfigBuilder.js is loaded.
                        </div>
                    `;
                }
            }
        }, 0);

        return section.outerHTML;
    }

    /**
     * Create FHIR Validation configuration UI
     * Exposes all 4 backend config options with clear descriptions
     */
    createFHIRValidationUI(step) {
        if (!step.config) step.config = {};

        // Read current config — defaults match fhir_validation_executor.go
        const validationLevel = (step.config.validation_level || 'standard').toLowerCase();
        const fhirVersion     = step.config.fhir_version || 'R4';
        const profile         = step.config.profile || 'base';
        const requiredResources = step.config.required_resources || [];
        const validateReferences = step.config.validate_references !== false;
        const failOnError        = step.config.fail_on_error === true;

        // 14 resource types with hardcoded required field checks in the backend (line 304-319)
        const commonResources = [
            { value: 'Patient', desc: 'Demographics and administrative information' },
            { value: 'Encounter', desc: 'Interaction between patient and provider' },
            { value: 'Observation', desc: 'Measurements, lab results, vital signs' },
            { value: 'Condition', desc: 'Clinical conditions, diagnoses' },
            { value: 'Procedure', desc: 'Actions performed on patient' },
            { value: 'AllergyIntolerance', desc: 'Allergy or intolerance record' },
            { value: 'DiagnosticReport', desc: 'Diagnostic test findings' },
            { value: 'MedicationRequest', desc: 'Medication prescriptions/orders' },
            { value: 'Immunization', desc: 'Vaccination records' },
            { value: 'Coverage', desc: 'Insurance/payment coverage' },
            { value: 'MessageHeader', desc: 'Message routing metadata' },
            { value: 'Practitioner', desc: 'Healthcare provider information' },
            { value: 'Organization', desc: 'Organization details' },
            { value: 'Location', desc: 'Physical location information' }
        ];

        const levelDescriptions = {
            basic:    'Checks only that each resource has a known FHIR <code>resourceType</code>. Fastest — use for format-only pipelines.',
            standard: 'Basic + required-field cardinality from compiled R4 profiles (146 resource types) + internal bundle reference resolution. Recommended for most pipelines.',
            strict:   'Standard + terminology binding validation (required &amp; extensible ValueSets) + R4 constraint predicates (obs-6, bun-1, pat-1, …). Most thorough.'
        };

        // Build resource checkboxes
        let resourceCheckboxesHtml = commonResources.map(res => {
            const checked = requiredResources.includes(res.value) ? 'checked' : '';
            return `<label class="fhir-resource-checkbox" title="${res.desc}">
                <input type="checkbox" name="fhir_required_resource" value="${res.value}" ${checked}>
                <span class="fhir-resource-name">${res.value}</span>
                <span class="fhir-resource-desc">${res.desc}</span>
            </label>`;
        }).join('');

        // Custom resource chips (resources selected but not in the common 14)
        const commonValues = commonResources.map(r => r.value);
        const customResources = requiredResources.filter(r => !commonValues.includes(r));
        const customChipsHtml = customResources.map(r =>
            `<span class="fhir-custom-resource-chip" data-resource="${r}">
                ${r}
                <button type="button" class="remove-custom-resource-btn" title="Remove ${r}">&times;</button>
            </span>`
        ).join('');

        const isBasic = validationLevel === 'basic';
        const toggleOpacity = isBasic ? '0.5' : '1';

        const section = document.createElement('div');
        section.className = 'form-section';
        section.innerHTML = `
            <h4><i class="fas fa-shield-alt" style="color: var(--success-color, #28a745); margin-right: 6px;"></i>FHIR Validation Configuration</h4>

            <!-- Validation Level -->
            <div class="form-group">
                <label>Validation Level <span style="color: var(--danger-color, #dc3545);">*</span></label>
                <select id="fhirValidationLevel" name="fhir_validation_level" class="form-control">
                    <option value="basic" ${validationLevel === 'basic' ? 'selected' : ''}>Basic - Structure only</option>
                    <option value="standard" ${validationLevel === 'standard' ? 'selected' : ''}>Standard - Structure + required fields + references</option>
                    <option value="strict" ${validationLevel === 'strict' ? 'selected' : ''}>Strict - Full R4 schema compliance</option>
                </select>
                <div id="fhirLevelDescription" class="fhir-level-description">
                    ${levelDescriptions[validationLevel]}
                </div>
            </div>

            <!-- FHIR Version + Profile (two-column row) -->
            <div class="form-group" style="display: grid; grid-template-columns: 1fr 1fr; gap: 12px;">
                <div>
                    <label>FHIR Version</label>
                    <select id="fhirVersionSelect" class="form-control">
                        <option value="R4" ${fhirVersion === 'R4' ? 'selected' : ''}>R4 (default)</option>
                        <option value="R5" ${fhirVersion === 'R5' ? 'selected' : ''}>R5</option>
                    </select>
                </div>
                <div>
                    <label>Validation Profile</label>
                    <select id="fhirProfileSelect" class="form-control">
                        <option value="base"    ${profile === 'base'     ? 'selected' : ''}>Base R4</option>
                        <option value="us-core" ${profile === 'us-core'  ? 'selected' : ''}>US Core</option>
                    </select>
                </div>
            </div>

            <!-- Validate References -->
            <div class="form-group" id="fhirValidateReferencesGroup" style="opacity: ${toggleOpacity};">
                <label class="fhir-toggle-label">
                    <input type="checkbox" id="fhirValidateReferences" ${validateReferences ? 'checked' : ''}>
                    <span>Validate Internal References</span>
                </label>
                <small class="form-text text-muted" style="display: block; margin-top: 4px;">
                    Check that <code>reference</code> fields point to existing resources (by fullUrl or ResourceType/id). Most useful in bundle mode where references can be cross-checked. Applies at Standard and Strict levels.
                </small>
            </div>

            <!-- Required Resources -->
            <div class="form-group">
                <label>Required Resources</label>
                <small class="form-text text-muted" style="display: block; margin-bottom: 8px;">
                    Resource types that <strong>must</strong> be present. In bundle mode, checks all entries. In resource mode, checks the single resource type matches.
                </small>
                <div class="fhir-resource-grid" id="fhirResourceGrid">
                    ${resourceCheckboxesHtml}
                </div>

                <!-- Custom resource entry -->
                <div style="margin-top: 10px;">
                    <div class="fhir-custom-resource-chips" id="fhirCustomResourceChips">
                        ${customChipsHtml}
                    </div>
                    <div style="display: flex; gap: 8px; align-items: center; margin-top: 6px;">
                        <input type="text" id="fhirCustomResourceInput" class="form-control" placeholder="Other resource type (e.g., Specimen)"
                               style="flex: 1; font-size: 0.85rem;">
                        <button type="button" id="fhirAddCustomResource" class="btn btn-sm btn-outline-secondary" style="white-space: nowrap;">
                            + Add
                        </button>
                    </div>
                </div>
            </div>

            <!-- Fail on Error -->
            <div class="form-group" style="margin-top: 12px; padding: 10px 12px; background: ${failOnError ? '#fff5f5' : 'var(--bg-secondary, #f8f9fa)'}; border: 1px solid ${failOnError ? '#feb2b2' : 'var(--border-color, #dee2e6)'}; border-radius: 6px;">
                <label class="fhir-toggle-label">
                    <input type="checkbox" id="fhirFailOnError" ${failOnError ? 'checked' : ''}>
                    <span>Fail on Error</span>
                </label>
                <small class="form-text text-muted" style="display: block; margin-top: 4px;">
                    When enabled, the pipeline <strong>stops</strong> if any validation error is found. When disabled, errors are reported in the step output but the pipeline continues.
                </small>
            </div>

            <!-- Info box -->
            <div class="fhir-info-box">
                <div style="font-weight: 600; margin-bottom: 6px;">
                    <i class="fas fa-info-circle"></i> How it works
                </div>
                <div style="font-size: 0.82rem; line-height: 1.5;">
                    <strong>Input:</strong> Auto-detects FHIR data from <code>fhirBundle</code> or <code>fhirResource</code> keys. Works with both full Bundles and standalone resources.<br><br>
                    <strong>Basic:</strong> Only checks <code>resourceType</code> is a known FHIR type. Fastest.<br>
                    <strong>Standard:</strong> Basic + required-field cardinality from compiled R4 profiles (146 resource types) + internal Bundle reference resolution.<br>
                    <strong>Strict:</strong> Standard + terminology binding validation (required &amp; extensible ValueSets) + R4 constraint predicates (obs-6, bun-1, pat-1, …). Most thorough.
                </div>
            </div>
        `;

        // Attach events after DOM insertion
        setTimeout(() => {
            // Validation Level change → update description + toggle visibility
            const levelSelect = document.getElementById('fhirValidationLevel');
            const levelDesc = document.getElementById('fhirLevelDescription');
            const reqFieldsGroup = document.getElementById('fhirValidateRequiredFieldsGroup');
            const refsGroup = document.getElementById('fhirValidateReferencesGroup');

            if (levelSelect) {
                levelSelect.addEventListener('change', () => {
                    const level = levelSelect.value;
                    if (levelDesc) levelDesc.innerHTML = levelDescriptions[level] || '';
                    const dim = level === 'basic' ? '0.5' : '1';
                    if (reqFieldsGroup) reqFieldsGroup.style.opacity = dim;
                    if (refsGroup) refsGroup.style.opacity = dim;
                });
            }

            // Custom resource Add button
            const addBtn = document.getElementById('fhirAddCustomResource');
            const customInput = document.getElementById('fhirCustomResourceInput');
            const chipsContainer = document.getElementById('fhirCustomResourceChips');

            if (addBtn && customInput && chipsContainer) {
                const addCustomResource = () => {
                    const value = customInput.value.trim();
                    if (!value) return;

                    // PascalCase validation
                    if (!/^[A-Z][a-zA-Z]+$/.test(value)) {
                        customInput.style.borderColor = 'var(--danger-color, #dc3545)';
                        setTimeout(() => { customInput.style.borderColor = ''; }, 2000);
                        return;
                    }

                    // Check if already in grid checkboxes
                    const existingCheckbox = document.querySelector(`input[name="fhir_required_resource"][value="${value}"]`);
                    if (existingCheckbox) {
                        existingCheckbox.checked = true;
                        customInput.value = '';
                        return;
                    }
                    // Check if already a custom chip
                    if (chipsContainer.querySelector(`[data-resource="${value}"]`)) {
                        customInput.value = '';
                        return;
                    }

                    // Add chip
                    const chip = document.createElement('span');
                    chip.className = 'fhir-custom-resource-chip';
                    chip.dataset.resource = value;
                    chip.innerHTML = `${value} <button type="button" class="remove-custom-resource-btn" title="Remove ${value}">&times;</button>`;
                    chip.querySelector('.remove-custom-resource-btn').addEventListener('click', () => chip.remove());
                    chipsContainer.appendChild(chip);
                    customInput.value = '';
                };

                addBtn.addEventListener('click', addCustomResource);
                customInput.addEventListener('keydown', (e) => {
                    if (e.key === 'Enter') {
                        e.preventDefault();
                        addCustomResource();
                    }
                });
            }

            // Attach remove handlers for existing custom chips
            if (chipsContainer) {
                chipsContainer.querySelectorAll('.remove-custom-resource-btn').forEach(btn => {
                    btn.addEventListener('click', () => btn.closest('.fhir-custom-resource-chip').remove());
                });
            }

            // Fail on Error toggle — visual feedback
            const failToggle = document.getElementById('fhirFailOnError');
            if (failToggle) {
                failToggle.addEventListener('change', () => {
                    const container = failToggle.closest('.form-group');
                    if (container) {
                        container.style.background = failToggle.checked ? '#fff5f5' : 'var(--bg-secondary, #f8f9fa)';
                        container.style.borderColor = failToggle.checked ? '#feb2b2' : 'var(--border-color, #dee2e6)';
                    }
                });
            }
        }, 0);

        return section.outerHTML;
    }

    /**
     * Handle transformation selection - show input fields for complex transforms
     */
    handleTransformSelection() {
        const transformSelect = document.getElementById('newMappingTransforms');
        const selectedValues = Array.from(transformSelect.selectedOptions).map(opt => opt.value);

        // Show/hide regex input
        const regexInput = document.getElementById('regexInput');
        if (selectedValues.includes('regex')) {
            regexInput.style.display = 'block';
            document.getElementById('regexPattern').focus();
        } else {
            regexInput.style.display = 'none';
            document.getElementById('regexPattern').value = '';
        }

        // Show/hide substring input
        const substringInput = document.getElementById('substringInput');
        if (selectedValues.includes('substring')) {
            substringInput.style.display = 'block';
            document.getElementById('substringParams').focus();
        } else {
            substringInput.style.display = 'none';
            document.getElementById('substringParams').value = '';
        }

        // Show/hide replace input
        const replaceInput = document.getElementById('replaceInput');
        if (selectedValues.includes('replace')) {
            replaceInput.style.display = 'block';
            document.getElementById('replaceParams').focus();
        } else {
            replaceInput.style.display = 'none';
            document.getElementById('replaceParams').value = '';
        }
    }

    /**
     * Add complex transform (regex, substring, replace) - deprecated, using handleTransformSelection instead
     */
    addComplexTransform(type) {
        // This method is kept for backward compatibility with the buttons in helper text
        // But the main interaction is now through the dropdown onchange
        const transformSelect = document.getElementById('newMappingTransforms');
        const option = Array.from(transformSelect.options).find(opt => opt.value === type);
        if (option && !option.selected) {
            option.selected = true;
            this.handleTransformSelection();
        }
    }

    /**
     * Save generic mapping (from inline form)
     */
    saveGenericMapping() {
        // Ensure step and config exist
        if (!this.currentStep) {
            this.builder.dragDropManager.showNotification('No step selected', 'error');
            return;
        }

        if (!this.currentStep.config) {
            this.currentStep.config = {};
        }
        if (!this.currentStep.config.mappings) {
            this.currentStep.config.mappings = [];
        }

        const lhs = document.getElementById('newMappingLHS').value.trim();
        const rhs = document.getElementById('newMappingRHS').value.trim();

        // Get selected transformations from multi-select
        const transformSelect = document.getElementById('newMappingTransforms');
        const selectedOptions = Array.from(transformSelect.selectedOptions).map(opt => opt.value);

        // Build final transform list, replacing simple regex/substring/replace with parameterized versions
        const finalTransforms = [];
        const regexPattern = document.getElementById('regexPattern').value.trim();
        const substringParams = document.getElementById('substringParams').value.trim();
        const replaceParams = document.getElementById('replaceParams').value.trim();

        selectedOptions.forEach(transform => {
            if (transform === 'regex' && regexPattern) {
                finalTransforms.push(`regex:${regexPattern}`);
            } else if (transform === 'substring' && substringParams) {
                finalTransforms.push(`substring:${substringParams}`);
            } else if (transform === 'replace' && replaceParams) {
                finalTransforms.push(`replace:${replaceParams}`);
            } else if (transform !== 'regex' && transform !== 'substring' && transform !== 'replace') {
                // Simple transforms (trim, upper, lower)
                finalTransforms.push(transform);
            }
        });

        const transforms = finalTransforms.join(', ');

        if (!lhs || !rhs) {
            this.builder.dragDropManager.showNotification('LHS and RHS are required', 'error');
            return;
        }

        const mappingObject = {
            lhs: lhs,
            rhs: rhs,
            transforms: transforms || ''
        };

        // Add new mapping
        this.currentStep.config.mappings.push(mappingObject);

        console.log('[Field Mapping] Variable added:', mappingObject);
        console.log('[Field Mapping] Total mappings:', this.currentStep.config.mappings.length);
        console.log('[Field Mapping] Step ID:', this.currentStep.id);

        // Clear form
        document.getElementById('newMappingLHS').value = '';
        document.getElementById('newMappingRHS').value = '';
        transformSelect.selectedIndex = -1;
        document.getElementById('regexPattern').value = '';
        document.getElementById('substringParams').value = '';
        document.getElementById('replaceParams').value = '';
        document.getElementById('regexInput').style.display = 'none';
        document.getElementById('substringInput').style.display = 'none';
        document.getElementById('replaceInput').style.display = 'none';

        // Update step in pipeline model and auto-save immediately
        this.builder.updateStep(this.currentStep);
        this.builder.savePipeline().then(() => {
            console.log('[Field Mapping] Pipeline auto-saved after adding variable');
        }).catch(err => {
            console.error('[Field Mapping] Auto-save failed:', err);
            this.builder.dragDropManager.showNotification('Warning: Variable added but save failed. Click Save Pipeline manually.', 'warning');
        });

        this._refreshMappingPanel();
        this.builder.dragDropManager.showNotification('Variable added & saved', 'success');
        this.builder.markAsUnsaved();
    }

    /**
     * Render API Response Mapping Builder UI
     * Shows configured output variable mappings from API response
     */
    renderApiResponseMappingBuilder(container, initialMappings, step) {
        const extractors = initialMappings.extractors || [];

        let html = `
            <div class="api-response-mapping-builder">
                <div class="mapping-header" style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px;">
                    <span style="font-size: 12px; color: #666;">
                        ${extractors.length} output variable${extractors.length !== 1 ? 's' : ''} configured
                    </span>
                    <button type="button" class="add-mapping-btn" style="
                        padding: 4px 10px;
                        background: #e3f2fd;
                        border: 1px solid #90caf9;
                        border-radius: 4px;
                        cursor: pointer;
                        font-size: 12px;
                    ">+ Add Variable</button>
                </div>
        `;

        if (extractors.length === 0) {
            html += `
                <div class="no-mappings" style="
                    padding: 20px;
                    background: #f5f5f5;
                    border-radius: 6px;
                    text-align: center;
                    color: #888;
                    font-style: italic;
                ">
                    <p style="margin: 0 0 10px 0;">No output variables configured.</p>
                    <p style="margin: 0; font-size: 12px;">Use "Test API Endpoint" above to test your API and click response fields to add them as output variables.</p>
                </div>
            `;
        } else {
            html += `<div class="mapping-list" style="display: flex; flex-direction: column; gap: 8px;">`;

            extractors.forEach((extractor, index) => {
                html += `
                    <div class="mapping-item" data-index="${index}" style="
                        display: flex;
                        align-items: center;
                        gap: 8px;
                        padding: 8px 12px;
                        background: #fff;
                        border: 1px solid #e0e0e0;
                        border-radius: 6px;
                    ">
                        <div class="mapping-source" style="flex: 1; min-width: 0;">
                            <label style="font-size: 10px; color: #888; display: block;">Source Path</label>
                            <input type="text" class="extractor-source-path" value="${this.builder.escapeHtml(extractor.sourcePath || '')}"
                                   style="width: 100%; padding: 4px 8px; border: 1px solid #ddd; border-radius: 4px; font-size: 12px;"
                                   placeholder="response.data.field">
                        </div>
                        <span style="color: #666; font-size: 16px;">→</span>
                        <div class="mapping-target" style="flex: 1; min-width: 0;">
                            <label style="font-size: 10px; color: #888; display: block;">Output Variable</label>
                            <input type="text" class="extractor-target-field" value="${this.builder.escapeHtml(extractor.targetField || '')}"
                                   style="width: 100%; padding: 4px 8px; border: 1px solid #ddd; border-radius: 4px; font-size: 12px;"
                                   placeholder="variableName">
                        </div>
                        <button type="button" class="delete-mapping-btn" data-index="${index}" style="
                            padding: 4px 8px;
                            background: none;
                            border: none;
                            color: #999;
                            cursor: pointer;
                            font-size: 16px;
                        " title="Remove mapping">×</button>
                    </div>
                `;
            });

            html += `</div>`;
        }

        html += `</div>`;

        container.innerHTML = html;

        // Attach events
        const addBtn = container.querySelector('.add-mapping-btn');
        if (addBtn) {
            addBtn.addEventListener('click', () => {
                if (!step.config.responseMapping) {
                    step.config.responseMapping = { mode: 'custom', extractors: [] };
                }
                step.config.responseMapping.extractors.push({
                    sourcePath: '',
                    targetField: '',
                    transformType: 'none',
                    required: false
                });
                this.renderApiResponseMappingBuilder(container, step.config.responseMapping, step);
                this.builder.markAsUnsaved();
            });
        }

        // Attach input change events
        container.querySelectorAll('.mapping-item').forEach((item, index) => {
            const sourceInput = item.querySelector('.extractor-source-path');
            const targetInput = item.querySelector('.extractor-target-field');
            const deleteBtn = item.querySelector('.delete-mapping-btn');

            if (sourceInput) {
                sourceInput.addEventListener('change', () => {
                    if (step.config.responseMapping?.extractors?.[index]) {
                        step.config.responseMapping.extractors[index].sourcePath = sourceInput.value;
                        this.builder.markAsUnsaved();
                    }
                });
            }

            if (targetInput) {
                targetInput.addEventListener('change', () => {
                    if (step.config.responseMapping?.extractors?.[index]) {
                        step.config.responseMapping.extractors[index].targetField = targetInput.value;
                        this.builder.markAsUnsaved();
                    }
                });
            }

            if (deleteBtn) {
                deleteBtn.addEventListener('click', () => {
                    if (step.config.responseMapping?.extractors) {
                        step.config.responseMapping.extractors.splice(index, 1);
                        this.renderApiResponseMappingBuilder(container, step.config.responseMapping, step);
                        this.builder.markAsUnsaved();
                    }
                });
            }
        });

        // Store reference
        container._apiResponseMappingBuilderInstance = { getMappings: () => step.config.responseMapping };
    }

    /**
     * Update mapping field inline (new feature for better UX)
     */
    updateMapping(index, field, value) {
        if (!this.currentStep || !this.currentStep.config || !this.currentStep.config.mappings) {
            return;
        }

        const mapping = this.currentStep.config.mappings[index];
        if (!mapping) return;

        mapping[field] = value;
        this.builder.markAsUnsaved();

        console.log(`[Field Mapping] Updated ${field} for mapping ${index}:`, value);

        // Auto-save after inline edit (debounced via pipeline save mechanism)
        this.builder.updateStep(this.currentStep);
        this.builder.savePipeline().then(() => {
            console.log('[Field Mapping] Pipeline auto-saved after mapping edit');
        }).catch(err => {
            console.error('[Field Mapping] Auto-save failed:', err);
        });
    }

    async deleteGenericMapping(index) {
        const confirmed = await this.builder.dragDropManager.showConfirmDialog(
            'Are you sure you want to delete this mapping?',
            {
                title: 'Delete Mapping',
                confirmText: 'Delete',
                cancelText: 'Cancel',
                type: 'danger'
            }
        );

        if (!confirmed) {
            return;
        }

        this.currentStep.config.mappings.splice(index, 1);
        this.builder.dragDropManager.showNotification('Mapping deleted & saved', 'success');
        this._refreshMappingPanel();
        this.builder.markAsUnsaved();

        // Auto-save after deletion
        this.builder.updateStep(this.currentStep);
        this.builder.savePipeline().then(() => {
            console.log('[Field Mapping] Pipeline auto-saved after mapping deletion');
        }).catch(err => {
            console.error('[Field Mapping] Auto-save failed:', err);
        });
    }

    /**
     * Add system variable quick-add
     */
    addSystemVariable(type) {
        if (!this.currentStep) return;
        if (!this.currentStep.config) this.currentStep.config = {};
        if (!this.currentStep.config.mappings) this.currentStep.config.mappings = [];

        let lhs, rhs, transforms = '';

        switch(type) {
            case 'timestamp':
                lhs = 'receivedAt';
                rhs = '${CURRENT_TIMESTAMP}';
                break;
            case 'correlationId':
                lhs = 'correlationId';
                rhs = '${GUID}';
                break;
            case 'messageId':
                lhs = 'messageId';
                rhs = '${UUID}';
                break;
            case 'interfaceId':
                lhs = 'interfaceId';
                rhs = '${INTERFACE_ID}';
                break;
            case 'interfaceName':
                lhs = 'interfaceName';
                rhs = '${INTERFACE_NAME}';
                break;
            default:
                return;
        }

        // Check if variable already exists
        const exists = this.currentStep.config.mappings.some(m => m.lhs === lhs);
        if (exists) {
            this.builder.dragDropManager.showNotification(`Variable "${lhs}" already exists`, 'warning');
            return;
        }

        const mappingObject = { lhs, rhs, transforms };
        this.currentStep.config.mappings.push(mappingObject);

        console.log('[Field Mapping] System variable added:', mappingObject);

        // Auto-save immediately
        this.builder.updateStep(this.currentStep);
        this.builder.savePipeline().then(() => {
            console.log('[Field Mapping] Pipeline auto-saved after system variable');
        }).catch(err => {
            console.error('[Field Mapping] Auto-save failed:', err);
        });

        this._refreshMappingPanel();
        this.builder.dragDropManager.showNotification(`Added system variable: ${lhs}`, 'success');
        this.builder.markAsUnsaved();
    }

    /**
     * Edit a mapping (single-click edit)
     */
    editMapping(index) {
        // Ensure step and config exist
        if (!this.currentStep) {
            this.builder.dragDropManager.showNotification('No step selected', 'error');
            return;
        }

        // Initialize config if needed
        if (!this.currentStep.config) {
            this.currentStep.config = {};
        }
        if (!this.currentStep.config.mappings) {
            this.currentStep.config.mappings = [];
        }

        // Get mapping (undefined for new mapping).
        // In interface_ref mode config.mappings is empty — the rendered table is backed
        // by _displayedTemplateMappings fetched from the backend, so fall back to that.
        const mapping = index !== undefined
            ? (this.currentStep.config.mappings[index] || this._displayedTemplateMappings?.[index] || {})
            : {};

        // Create edit modal with NO-CODE enhancements
        const editModalHTML = `
            <div id="editMappingModal" class="modal" style="display: flex;">
                <div class="modal-content" style="max-width: 700px;">
                    <div class="modal-header" style="background: linear-gradient(135deg, #1e3a8a 0%, #2563eb 100%); color: white;">
                        <h3><i class="fas fa-edit"></i> ${index === undefined ? 'Add' : 'Edit'} Field Mapping</h3>
                        <button class="modal-close" onclick="document.getElementById('editMappingModal').remove()">&times;</button>
                    </div>
                    <div class="modal-body">
                        <!-- NO-CODE Info Box -->
                        <div style="margin-bottom: 1.5rem; padding: 0.75rem; background: linear-gradient(135deg, #10b981 0%, #059669 100%); border-radius: 6px; color: white; font-size: 0.85rem;">
                            <div style="display: flex; align-items: start; gap: 0.5rem;">
                                <div style="font-size: 1.2rem;">💡</div>
                                <div>
                                    <strong>NO-CODE Mapping:</strong> Use dropdowns and buttons to select fields - no manual typing needed!
                                    <br>Click <strong>"📚 Browse Variables"</strong> to use enriched data from previous steps.
                                </div>
                            </div>
                        </div>

                        <!-- Source Field (HL7 or Enriched Data) -->
                        <div class="form-group">
                            <label style="display: flex; align-items: center; gap: 0.5rem; margin-bottom: 0.5rem;">
                                <span style="font-weight: 600;">Source Field</span>
                                <small style="color: #64748b;">(HL7 field or enriched data)</small>
                            </label>
                            <div style="display: flex; gap: 0.5rem; margin-bottom: 0.5rem;">
                                <select id="editSourceType" style="padding: 0.5rem; border: 1px solid #cbd5e1; border-radius: 4px; background: white; flex: 0 0 140px;" onchange="window.propertiesPanel.toggleSourceInput()">
                                    <option value="hl7">HL7 Field</option>
                                    <option value="enriched">Enriched Data</option>
                                    <option value="custom">Custom XPath</option>
                                    <option value="static_value">Static Value</option>
                                </select>
                                <button class="btn btn-secondary" style="font-size: 0.85rem; padding: 0.5rem 0.75rem; flex-shrink: 0;"
                                        onclick="window.propertiesPanel.switchToManualEntry()"
                                        title="Type the field path manually">
                                    <i class="fas fa-pencil-alt"></i> Type Manually
                                </button>
                            </div>

                            <!-- HL7 Field Dropdown -->
                            <select id="editHl7FieldDropdown" style="width: 100%; padding: 0.5rem; border: 1px solid #cbd5e1; border-radius: 4px; background: white; display: block;">
                                <option value="">-- Select HL7 Field --</option>
                                <optgroup label="Patient Demographics (PID)">
                                    <option value="PID.3">PID.3 - Patient ID</option>
                                    <option value="PID.5">PID.5 - Patient Name</option>
                                    <option value="PID.7">PID.7 - Date of Birth</option>
                                    <option value="PID.8">PID.8 - Gender</option>
                                    <option value="PID.11">PID.11 - Patient Address</option>
                                    <option value="PID.13">PID.13 - Phone Number</option>
                                    <option value="PID.16">PID.16 - Marital Status</option>
                                </optgroup>
                                <optgroup label="Visit Information (PV1)">
                                    <option value="PV1.2">PV1.2 - Patient Class</option>
                                    <option value="PV1.3">PV1.3 - Assigned Location</option>
                                    <option value="PV1.7">PV1.7 - Attending Doctor</option>
                                    <option value="PV1.8">PV1.8 - Referring Doctor</option>
                                    <option value="PV1.19">PV1.19 - Visit Number</option>
                                    <option value="PV1.44">PV1.44 - Admit Date/Time</option>
                                </optgroup>
                                <optgroup label="Observations (OBX)">
                                    <option value="OBX.3">OBX.3 - Observation Identifier</option>
                                    <option value="OBX.5">OBX.5 - Observation Value</option>
                                    <option value="OBX.6">OBX.6 - Units</option>
                                    <option value="OBX.7">OBX.7 - Reference Range</option>
                                    <option value="OBX.11">OBX.11 - Observation Status</option>
                                </optgroup>
                            </select>

                            <!-- Custom Text Input (hidden by default) -->
                            <!-- data-hl7dtype carries the HL7 data type for composite detection -->
                            <input type="text" id="editHl7Field"
                                value="${mapping.hl7Field || mapping.sourceField || mapping.sourcePath || ''}"
                                data-hl7dtype="${mapping.hl7DataType || ''}"
                                style="width: 100%; padding: 0.5rem; border: 1px solid #cbd5e1; border-radius: 4px; display: none;"
                                placeholder="e.g., PID.5 or [\\"Step Name\\"].enriched_data.fieldName">
                            <!-- Hidden: current resource type, used by CompositeTypePicker -->
                            <input type="hidden" id="editResourceType"
                                value="${mapping.resourceType || mapping.fhirResourceType || ''}">
                            <!-- Static Value Input (hidden by default) -->
                            <input type="text" id="editStaticValue" value="${mapping.staticValue || mapping.defaultValue || ''}"
                                style="width: 100%; padding: 0.5rem; border: 1px solid #c6f6d5; border-radius: 4px; display: none; background: #f0fff4;"
                                placeholder="Literal value, e.g.  group  or  active  or  true">
                            <small id="editSourceTip" style="color: #64748b; font-size: 0.8rem; margin-top: 0.25rem; display: block;">
                                💡 Tip: Select from dropdown, type manually, browse the payload below, or choose Static Value for a literal constant
                            </small>

                            <!-- HL7 Payload Browser -->
                            <div style="margin-top: 0.6rem;">
                                <button type="button" id="hl7BrowserToggle"
                                    onclick="window.propertiesPanel.toggleHL7Browser()"
                                    style="width:100%; text-align:left; padding:0.45rem 0.75rem; background:#f0f9ff; border:1px solid #bae6fd; border-radius:6px; cursor:pointer; font-size:0.82rem; font-weight:600; color:#0369a1; display:flex; align-items:center; gap:0.5rem;">
                                    <i class="fas fa-sitemap" style="font-size:0.78rem;"></i>
                                    Browse fields from HL7 payload
                                    <i class="fas fa-chevron-right" id="hl7BrowserChevron" style="margin-left:auto; font-size:0.72rem; transition:transform 0.2s;"></i>
                                </button>
                                <div id="hl7BrowserBody" style="display:none; margin-top:0.25rem; border:1px solid #bae6fd; border-radius:6px; overflow:hidden;">
                                    <div style="padding:0.5rem; border-bottom:1px solid #e0f2fe; background:#f0f9ff;">
                                        <input type="text" id="hl7BrowserSearch" placeholder="🔍 Filter fields (e.g. PID, ZIN, patient)"
                                            style="width:100%; padding:0.35rem 0.6rem; border:1px solid #bae6fd; border-radius:4px; font-size:0.82rem; outline:none; box-sizing:border-box;"
                                            oninput="window.propertiesPanel.filterHL7Browser(this.value)">
                                    </div>
                                    <div id="hl7BrowserContent" style="max-height:260px; overflow-y:auto; background:#fff;">
                                        <div style="padding:1rem; text-align:center; color:#94a3b8; font-size:0.85rem;">
                                            <i class="fas fa-spinner fa-spin"></i> Loading payload structure…
                                        </div>
                                    </div>
                                </div>
                            </div>
                        </div>

                        <!-- Target Field (FHIR Path) -->
                        <div class="form-group">
                            <label style="display:flex; align-items:center; gap:0.5rem; margin-bottom:0.5rem;">
                                <span style="font-weight:600;">FHIR Target Path</span>
                                <button type="button"
                                    onclick="window.propertiesPanel._openExtensionBuilder()"
                                    style="margin-left:auto;padding:3px 10px;background:#ede9fe;border:1px solid #c4b5fd;border-radius:5px;
                                           cursor:pointer;font-size:0.75rem;font-weight:600;color:#5b21b6;white-space:nowrap;"
                                    title="Open guided extension mapping popup">
                                    🔌 Extension
                                </button>
                            </label>

                            <!-- Composite type picker — shown when HL7 source is a composite type.
                                 Hidden for primitive types; CompositeTypePicker renders into this div. -->
                            <div id="compositeModeContainer" style="display:none; margin-bottom:0.5rem;"></div>

                            <!-- Simple path input — hidden while composite picker is active -->
                            <div id="fhirSimpleSection" style="position:relative;">
                                <i class="fas fa-search" style="position:absolute; left:0.6rem; top:50%; transform:translateY(-50%); color:#94a3b8; font-size:0.78rem; pointer-events:none;"></i>
                                <input type="text" id="editFhirPath"
                                    value="${mapping.fhirPath || mapping.targetField || mapping.targetPath || ''}"
                                    placeholder="Search or type path (e.g. Coverage.subscriberId)"
                                    autocomplete="off"
                                    style="width:100%; padding:0.5rem 0.75rem 0.5rem 2rem; border:1px solid #cbd5e1; border-radius:4px; box-sizing:border-box; font-size:0.875rem;"
                                    oninput="window.propertiesPanel._filterFhirDropdown(this.value)"
                                    onfocus="window.propertiesPanel._showFhirDropdown()"
                                    onblur="setTimeout(() => window.propertiesPanel._hideFhirDropdown(), 200)"
                                    onkeydown="window.propertiesPanel._fhirDropdownKeydown(event)">
                                <div id="editFhirSuggestions"
                                    style="display:none; position:absolute; top:calc(100% + 2px); left:0; right:0; z-index:2000;
                                           background:white; border:1px solid #cbd5e1; border-radius:4px;
                                           box-shadow:0 4px 16px rgba(0,0,0,0.12); max-height:240px; overflow-y:auto;">
                                </div>
                            </div>
                            <small style="color:#64748b; font-size:0.8rem; margin-top:0.25rem; display:block;">
                                💡 Select from list (built from this interface's mappings) or type a custom path
                            </small>
                        </div>

                        <!-- Transform Type -->
                        <div class="form-group">
                            <label style="font-weight: 600; margin-bottom: 0.5rem; display: block;">
                                Transform
                                <small style="font-weight:400; color:#64748b; margin-left:0.4rem;">(how the HL7 value is converted)</small>
                            </label>
                            <select id="editTransformType"
                                data-current="${mapping.transformType || mapping.dataTypeTransform || ''}"
                                style="width: 100%; padding: 0.5rem; border: 1px solid #cbd5e1; border-radius: 4px; background: white;">
                                <option value="">Auto (engine selects based on data types)</option>
                                <option value="ce_to_codeableconcept">CE → CodeableConcept</option>
                                <option value="cx_to_identifier">CX → Identifier</option>
                                <option value="xpn_to_humanname">XPN → HumanName</option>
                                <option value="xad_to_address">XAD → Address</option>
                                <option value="xtn_to_contactpoint">XTN → ContactPoint</option>
                                <option value="ts_to_datetime">TS → dateTime</option>
                                <option value="ts_to_date">TS → date</option>
                                <option value="gender_mapping">Gender (M/F/U → male/female/unknown)</option>
                                <option value="hl7_active_flag">Y/N → Boolean (active flag)</option>
                                <option value="msh9_trigger_event_to_coding">MSH-9 → EventCoding</option>
                                <option value="obx_value_by_type">OBX value by type</option>
                            </select>
                            <small style="color:#64748b; font-size:0.8rem; margin-top:0.25rem; display:block;">
                                💡 For ZPD / custom segments: pick <strong>Y/N → Boolean</strong> for flag fields, or leave Auto
                            </small>
                        </div>

                        <!-- Data Type -->
                        <div class="form-group">
                            <label style="font-weight: 600; margin-bottom: 0.5rem; display: block;">Data Type <small style="font-weight:400; color:#64748b;">(optional hint)</small></label>
                            <select id="editDataTypeDropdown" style="width: 100%; padding: 0.5rem; border: 1px solid #cbd5e1; border-radius: 4px; background: white;" onchange="document.getElementById('editDataType').value = this.value">
                                <option value="">-- Select Data Type --</option>
                                <option value="string">string - Text value</option>
                                <option value="integer">integer - Whole number</option>
                                <option value="decimal">decimal - Decimal number</option>
                                <option value="boolean">boolean - True/False</option>
                                <option value="dateTime">dateTime - Date and time</option>
                                <option value="date">date - Date only</option>
                                <option value="code">code - Coded value</option>
                                <option value="uri">uri - URI/URL</option>
                            </select>
                            <input type="hidden" id="editDataType" value="${mapping.dataType || mapping.hl7DataType || ''}">
                        </div>
                    </div>
                    <div class="modal-footer">
                        <button class="btn btn-secondary" onclick="document.getElementById('editMappingModal').remove()">Cancel</button>
                        <button class="btn btn-primary" onclick="window.propertiesPanel.saveEditedMapping(${index})">
                            <i class="fas fa-save"></i> ${index === undefined ? 'Add Mapping' : 'Save Changes'}
                        </button>
                    </div>
                </div>
            </div>
        `;

        // Remove existing edit modal if any
        const existingModal = document.getElementById('editMappingModal');
        if (existingModal) existingModal.remove();

        // Reset browser state so each new modal open loads fresh content
        this._hl7BrowserLoaded = false;

        // Add to DOM
        document.body.insertAdjacentHTML('beforeend', editModalHTML);

        // Close on overlay click
        const modal = document.getElementById('editMappingModal');
        modal.addEventListener('click', (e) => {
            if (e.target === modal) {
                modal.remove();
            }
        });

        // Initialize dropdown event handlers
        this.initializeMappingModalHandlers();
    }

    /**
     * Initialize mapping modal event handlers (NO-CODE features)
     */
    initializeMappingModalHandlers() {
        const hl7Dropdown  = document.getElementById('editHl7FieldDropdown');
        const hl7TextInput = document.getElementById('editHl7Field');
        const sourceType   = document.getElementById('editSourceType');

        // ── Pre-populate source field ─────────────────────────────────────────
        const staticInput = document.getElementById('editStaticValue');

        // Check for static_value mapping first (transformType == "static_value" or
        // sourcePath is empty but staticValue/defaultValue is populated).
        const existingTransform = hl7TextInput?.closest('.modal-body')
            ? document.getElementById('editDataType')?.value : '';

        const mappingIsStatic = (existingTransform === 'static_value') ||
            (staticInput?.value && !hl7TextInput?.value);

        if (mappingIsStatic && staticInput?.value) {
            sourceType.value           = 'static_value';
            hl7Dropdown.style.display  = 'none';
            hl7TextInput.style.display = 'none';
            staticInput.style.display  = 'block';
            const browser = document.getElementById('hl7PayloadBrowser');
            if (browser) browser.style.display = 'none';
        } else if (hl7TextInput && hl7TextInput.value) {
            const val = hl7TextInput.value;

            if (val.startsWith('[')) {
                // Enriched data reference e.g. ["Step"].enriched_data.field
                sourceType.value = 'enriched';
                hl7Dropdown.style.display  = 'none';
                hl7TextInput.style.display = 'block';
                hl7TextInput.placeholder   = 'e.g., ["database_enrichment"].enriched_data.fieldName';
            } else {
                // Standard HL7 path — try to find it in the dropdown
                const match = Array.from(hl7Dropdown.options).find(o => o.value === val);
                if (match) {
                    hl7Dropdown.value = val;
                    // Dropdown already visible; text input stays hidden
                } else {
                    // Not in the standard list (Z-segment, custom field) — show text input
                    sourceType.value           = 'custom';
                    hl7Dropdown.style.display  = 'none';
                    hl7TextInput.style.display = 'block';
                    hl7TextInput.placeholder   = 'e.g., ZIN.2 or PID.5.1';
                }
            }
        }

        // ── Pre-select transform type ─────────────────────────────────────────
        const transformSelect = document.getElementById('editTransformType');
        if (transformSelect && transformSelect.dataset.current) {
            transformSelect.value = transformSelect.dataset.current;
        }

        // ── Wire up dropdown → text input sync ───────────────────────────────
        if (hl7Dropdown) {
            hl7Dropdown.addEventListener('change', (e) => {
                if (e.target.value) hl7TextInput.value = e.target.value;
            });
        }

        // ── Build FHIR path suggestions from live template mappings ──────────
        // Sync build first (instant) using whatever is already in memory,
        // then async-enrich with OOB template so Coverage, Practitioner etc.
        // are always available even if not yet in this interface's mapping set.
        this._fhirPathGroups = this._buildFhirPathOptions();
        this._enrichFhirDropdownWithOOB();

        // ── Pre-populate Data Type dropdown ───────────────────────────────────
        const dataTypeDropdown = document.getElementById('editDataTypeDropdown');
        const existingDataType = document.getElementById('editDataType').value;
        if (dataTypeDropdown && existingDataType) {
            dataTypeDropdown.value = existingDataType;
        }

        // ── Composite type detection ──────────────────────────────────────────
        // When the HL7 source field has a known composite data type (XPN, XAD, …)
        // show the CompositeTypePicker in place of the plain FHIR path input.
        this._checkCompositeMode();
    }

    // ── Composite type mapping helpers ──────────────────────────────────────

    /**
     * Reads the current HL7 data type from the source field and activates the
     * CompositeTypePicker when the type is composite. Safe to call multiple times.
     */
    _checkCompositeMode() {
        if (typeof HL7TypeCatalog === 'undefined') return; // catalog not loaded yet

        const hl7Input   = document.getElementById('editHl7Field');
        const hl7DataType = (hl7Input && hl7Input.dataset.hl7dtype) || '';

        if (hl7DataType && HL7TypeCatalog.isComposite(hl7DataType)) {
            const hl7Field    = (hl7Input && hl7Input.value) || '';
            const resourceType = (document.getElementById('editResourceType') || {}).value || '';
            this._activateCompositePicker(hl7Field, hl7DataType, resourceType);
        } else {
            this._deactivateCompositePicker();
        }
    }

    /**
     * Shows the CompositeTypePicker, hides the plain FHIR path input.
     * Wires the 'mapping-selected' and 'composite-dismissed' events.
     */
    _activateCompositePicker(hl7Field, hl7DataType, resourceType) {
        const container    = document.getElementById('compositeModeContainer');
        const simpleSection = document.getElementById('fhirSimpleSection');
        if (!container) return;

        // Destroy any existing picker instance first
        if (this._compositePickerInstance) {
            this._compositePickerInstance.destroy();
            this._compositePickerInstance = null;
        }

        container.style.display     = '';
        if (simpleSection) simpleSection.style.display = 'none';

        const picker = new CompositeTypePicker(container, {
            hl7Field:     hl7Field,
            hl7DataType:  hl7DataType,
            resourceType: resourceType,
        });
        picker.render();
        this._compositePickerInstance = picker;

        // 'mapping-selected': populate all form fields from picker selection
        container.addEventListener('mapping-selected', (e) => {
            this._onCompositePickerSelect(e.detail);
        }, { once: false });

        // 'composite-dismissed': user clicked "Type manually" — fall back to input
        container.addEventListener('composite-dismissed', () => {
            this._deactivateCompositePicker();
        }, { once: true });
    }

    /**
     * Hides the CompositeTypePicker and restores the plain FHIR path input.
     */
    _deactivateCompositePicker() {
        const container     = document.getElementById('compositeModeContainer');
        const simpleSection = document.getElementById('fhirSimpleSection');

        if (this._compositePickerInstance) {
            this._compositePickerInstance.destroy();
            this._compositePickerInstance = null;
        }
        if (container)     container.style.display     = 'none';
        if (simpleSection) simpleSection.style.display = '';
    }

    /**
     * Handles a 'mapping-selected' event from the CompositeTypePicker.
     * Populates the HL7 source, FHIR target, and transform fields so the
     * existing saveEditedMapping() path works unchanged.
     */
    _onCompositePickerSelect(detail) {
        // Source field
        const hl7Input  = document.getElementById('editHl7Field');
        const srcType   = document.getElementById('editSourceType');
        const hl7Drop   = document.getElementById('editHl7FieldDropdown');
        if (hl7Input) {
            hl7Input.value              = detail.hl7Field || '';
            hl7Input.dataset.hl7dtype   = detail.hl7DataType || '';
        }
        // Switch source type to 'custom' so saveEditedMapping reads from the text input
        if (srcType)  srcType.value          = 'custom';
        if (hl7Drop)  hl7Drop.style.display  = 'none';
        if (hl7Input) hl7Input.style.display = 'block';

        // FHIR path — set the hidden input AND the visible input (for review)
        const fhirInput = document.getElementById('editFhirPath');
        if (fhirInput)  fhirInput.value = detail.fhirPath || '';

        // Transform type
        const transformSel = document.getElementById('editTransformType');
        if (transformSel) transformSel.value = detail.transformType || '';

        // Show a brief confirmation banner inside the composite container
        const container = document.getElementById('compositeModeContainer');
        if (container) {
            const banner = document.createElement('div');
            banner.style.cssText = 'margin-top:0.4rem;padding:0.4rem 0.75rem;background:#d1fae5;border:1px solid #6ee7b7;border-radius:5px;font-size:0.8rem;color:#065f46;display:flex;justify-content:space-between;align-items:center;';
            const esc = s => String(s).replace(/</g, '&lt;').replace(/>/g, '&gt;');
            banner.innerHTML = '<span>✓ Selected: <strong>' + esc(detail.hl7Field) + '</strong> → <strong>' + esc(detail.fhirPath) + '</strong></span>'
                + '<button type="button" style="background:none;border:none;cursor:pointer;font-size:0.8rem;color:#065f46;text-decoration:underline;" onclick="window.propertiesPanel._deactivateCompositePicker()">Change</button>';
            container.appendChild(banner);
        }
    }

    /**
     * Opens the ExtensionBuilder popup pre-loaded with the current HL7 source
     * field context. On confirm, populates fhirPath, transformType, and
     * deactivates the composite picker if active.
     */
    _openExtensionBuilder() {
        if (typeof ExtensionBuilder === 'undefined' || typeof FhirExtensionCatalog === 'undefined') {
            this.builder.dragDropManager.showNotification(
                'Extension Builder not loaded — refresh the page', 'error');
            return;
        }

        const hl7Input    = document.getElementById('editHl7Field');
        const resourceEl  = document.getElementById('editResourceType');
        const hl7Field    = (hl7Input && hl7Input.value) || '';
        const hl7DataType = (hl7Input && hl7Input.dataset.hl7dtype) || '';
        const resourceType = (resourceEl && resourceEl.value) || this._inferResourceType();

        const builder = new ExtensionBuilder({
            hl7Field:     hl7Field,
            hl7DataType:  hl7DataType,
            resourceType: resourceType,
            onConfirm: (result) => {
                // Populate FHIR path
                const fhirInput = document.getElementById('editFhirPath');
                if (fhirInput) fhirInput.value = result.fhirPath;

                // Set transform
                const transformSel = document.getElementById('editTransformType');
                if (transformSel) transformSel.value = result.transform || '';

                // Deactivate composite picker — extension path is now the target
                this._deactivateCompositePicker();

                this.builder.dragDropManager.showNotification(
                    '🔌 Extension path set — click Save to apply', 'success');
            },
        });
        builder.open();
    }

    /**
     * Re-renders the properties panel while preserving the mapping table's
     * scroll position. Use this everywhere instead of calling
     * showStepProperties(this.currentStep) directly after a mapping change,
     * so the user's scroll position survives the DOM rebuild.
     */
    _refreshMappingPanel() {
        const container = document.getElementById('mappingTableContainer');
        const scrollTop = container ? container.scrollTop : 0;

        this.showStepProperties(this.currentStep);

        // Restore scroll after the DOM has been fully rebuilt
        requestAnimationFrame(() => {
            const restored = document.getElementById('mappingTableContainer');
            if (restored && scrollTop > 0) {
                restored.scrollTop = scrollTop;
            }
        });
    }

    /**
     * Infers the FHIR resource type from the current step config or pipeline
     * when editResourceType is not explicitly set (e.g. on new mappings).
     */
    _inferResourceType() {
        // Try the current step's mapped resource types
        const step = this.currentStep;
        if (step && step.config && step.config.mappings && step.config.mappings.length > 0) {
            const rt = step.config.mappings[0].resourceType
                    || step.config.mappings[0].fhirResourceType;
            if (rt) return rt;
        }
        // Fall back to pipeline message type → default resource
        const messageType = this.builder.pipeline && this.builder.pipeline.messageType;
        if (messageType && messageType.startsWith('ADT')) return 'Patient';
        if (messageType && messageType.startsWith('ORU')) return 'Observation';
        return 'Patient';
    }

    /**
     * Switches the source input to manual text entry (for Z-segments or any path
     * not in the standard HL7 dropdown).
     */
    switchToManualEntry() {
        const sourceType   = document.getElementById('editSourceType');
        const hl7Dropdown  = document.getElementById('editHl7FieldDropdown');
        const hl7TextInput = document.getElementById('editHl7Field');
        if (!sourceType) return;

        sourceType.value           = 'custom';
        hl7Dropdown.style.display  = 'none';
        hl7TextInput.style.display = 'block';
        hl7TextInput.placeholder   = 'e.g., ZIN.2 or PID.5.1';
        hl7TextInput.focus();
    }

    // ── HL7 Payload Browser ────────────────────────────────────────────────────

    toggleHL7Browser() {
        const body    = document.getElementById('hl7BrowserBody');
        const chevron = document.getElementById('hl7BrowserChevron');
        if (!body) return;

        const opening = body.style.display === 'none';
        body.style.display    = opening ? 'block' : 'none';
        chevron.style.transform = opening ? 'rotate(90deg)' : '';

        // Load content the first time it's opened
        if (opening && !this._hl7BrowserLoaded) {
            this._hl7BrowserLoaded = true;
            this._loadHL7PayloadBrowser();
        }
    }

    async _loadHL7PayloadBrowser() {
        const content     = document.getElementById('hl7BrowserContent');
        if (!content) return;

        const interfaceId = this.currentStep?.config?.interface_id || this.builder.pipeline?.interfaceId;
        const messageType = this.currentStep?.config?.message_type || this.builder.pipeline?.messageType || 'ADT^A01';

        try {
            const params = new URLSearchParams({ messageType });
            if (interfaceId) params.set('interfaceId', interfaceId);

            const resp = await fetch(`/api/schemas/hl7/fields?${params}`, { credentials: 'include' });
            const data = await resp.json();

            if (!data.success || !data.xpathTree) {
                const messagesUrl = interfaceId
                    ? `/messages.html?interfaceId=${encodeURIComponent(interfaceId)}`
                    : '/messages.html';
                content.innerHTML = `
                    <div style="padding:1rem 1.25rem; font-size:0.82rem; color:#475569; line-height:1.7;">
                        <div style="display:flex; gap:0.6rem; align-items:flex-start; margin-bottom:0.75rem;">
                            <i class="fas fa-info-circle" style="color:#0369a1; margin-top:2px; flex-shrink:0;"></i>
                            <span>No sample message found yet. Run the pipeline with a test message — the field structure is captured automatically.</span>
                        </div>
                        <div style="font-weight:600; margin-bottom:0.4rem; color:#1e293b;">How to enable it:</div>
                        <ol style="margin:0 0 0.75rem 1.1rem; padding:0; display:flex; flex-direction:column; gap:0.3rem;">
                            <li>Close this modal</li>
                            <li>Click the <strong>Test Pipeline</strong> button on this page</li>
                            <li>Paste or type a sample HL7 message and run it</li>
                            <li>Reopen this edit modal — all segments including Z-segments will appear here</li>
                        </ol>
                    </div>`;
                return;
            }

            // xpathTree.children = segments
            this._hl7BrowserSegments = data.xpathTree.children || [];
            content.innerHTML = this._renderHL7BrowserSegments(this._hl7BrowserSegments);

        } catch (err) {
            content.innerHTML = `<div style="padding:1rem; text-align:center; color:#ef4444; font-size:0.82rem;">Failed to load: ${err.message}</div>`;
        }
    }

    _renderHL7BrowserSegments(segments) {
        if (!segments || segments.length === 0) return '<div style="padding:1rem; text-align:center; color:#94a3b8; font-size:0.82rem;">No segments found</div>';

        const esc = s => String(s || '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');

        return segments.map((seg, si) => {
            // Each segment's fields are inside seg.children[0] (the "fields" node)
            const fieldsNode = (seg.children || []).find(c => c.name === 'fields');
            const fieldNodes = (fieldsNode?.children || []).filter(f => f.type === 'field-value');

            const fieldRows = fieldNodes.map(f => {
                const subNodes = [];
                // Find matching field-object to get subfields
                const fObj = (fieldsNode?.children || []).find(c => c.type === 'field-object' && c.name.startsWith(f.name));
                const subfieldsNode = (fObj?.children || []).find(c => c.name === 'subfields');
                if (subfieldsNode) subNodes.push(...(subfieldsNode.children || []));

                const key  = esc(f.name);
                const desc = esc(f.description || '');
                const ex   = f.example != null ? esc(f.example) : '';

                const subRows = subNodes.map(sf => {
                    const sk  = esc(sf.name);
                    const sex = sf.example != null ? esc(sf.example) : '';
                    return `<div class="hl7-field-row hl7-subfield-row" data-hl7key="${sk}"
                                style="padding:0.28rem 0.75rem 0.28rem 2.5rem; display:flex; align-items:center; gap:0.5rem; cursor:pointer; border-bottom:1px solid #f1f5f9;"
                                onmouseover="this.style.background='#eff6ff'" onmouseout="this.style.background=''"
                                onclick="window.propertiesPanel.useHL7Field('${sk}')">
                        <code style="min-width:70px; font-size:0.75rem; color:#6d28d9; background:#ede9fe; padding:1px 5px; border-radius:3px;">${sk}</code>
                        <span style="flex:1; font-size:0.75rem; color:#475569; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">${sex ? `"${sex}"` : ''}</span>
                    </div>`;
                }).join('');

                return `<div class="hl7-field-row" data-hl7key="${key}"
                            style="padding:0.32rem 0.75rem; display:flex; align-items:center; gap:0.5rem; cursor:pointer; border-bottom:1px solid #f1f5f9;"
                            onmouseover="this.style.background='#eff6ff'" onmouseout="this.style.background=''"
                            onclick="window.propertiesPanel.useHL7Field('${key}')">
                    <code style="min-width:55px; font-size:0.78rem; color:#1e3a8a; background:#dbeafe; padding:1px 5px; border-radius:3px; flex-shrink:0;">${key}</code>
                    <span style="flex:1; font-size:0.78rem; color:#374151; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;" title="${desc}">${desc}</span>
                    ${ex ? `<span style="font-size:0.73rem; color:#64748b; max-width:90px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; flex-shrink:0;" title="${ex}">"${ex}"</span>` : ''}
                </div>${subRows}`;
            }).join('');

            const segName = esc(seg.name);
            const segDesc = esc(seg.description || '');
            const isZ     = seg.name.startsWith('Z');

            return `<div class="hl7-browser-segment" data-segment="${segName}">
                <div style="padding:0.4rem 0.75rem; background:${isZ ? '#fef9c3' : '#f8fafc'}; border-bottom:1px solid #e2e8f0;
                            display:flex; align-items:center; gap:0.5rem; cursor:pointer; user-select:none;"
                     onclick="this.nextElementSibling.style.display = this.nextElementSibling.style.display==='none' ? 'block' : 'none'">
                    <i class="fas fa-caret-right" style="font-size:0.7rem; color:#94a3b8;"></i>
                    <code style="font-size:0.8rem; font-weight:700; color:${isZ ? '#92400e' : '#1e3a8a'};">${segName}</code>
                    <span style="font-size:0.78rem; color:#64748b;">${segDesc}</span>
                    ${isZ ? '<span style="font-size:0.7rem; background:#fde68a; color:#92400e; padding:1px 5px; border-radius:3px; font-weight:600;">Z-seg</span>' : ''}
                    <span style="margin-left:auto; font-size:0.72rem; color:#94a3b8;">${fieldNodes.length} fields</span>
                </div>
                <div style="display:none;">${fieldRows}</div>
            </div>`;
        }).join('');
    }

    filterHL7Browser(query) {
        const q = query.trim().toLowerCase();
        document.querySelectorAll('.hl7-browser-segment').forEach(seg => {
            const rows = seg.querySelectorAll('.hl7-field-row');
            let anyMatch = false;
            rows.forEach(row => {
                const key  = (row.dataset.hl7key || '').toLowerCase();
                const text = (row.textContent || '').toLowerCase();
                const show = !q || key.includes(q) || text.includes(q);
                row.style.display = show ? '' : 'none';
                if (show) anyMatch = true;
            });
            const segName = (seg.dataset.segment || '').toLowerCase();
            seg.style.display = (!q || anyMatch || segName.includes(q)) ? '' : 'none';
            // Auto-expand segments that have matching fields
            if (q && anyMatch) {
                const body = seg.querySelector('div[style*="display"]');
                if (body) body.style.display = 'block';
            }
        });
    }

    /**
     * Called when a field row is clicked in the HL7 payload browser.
     * Populates the source field input and switches to manual entry mode.
     */
    useHL7Field(hl7Key) {
        const hl7TextInput = document.getElementById('editHl7Field');
        const sourceType   = document.getElementById('editSourceType');
        const hl7Dropdown  = document.getElementById('editHl7FieldDropdown');
        if (!hl7TextInput) return;

        // Switch to manual entry mode with the selected key
        sourceType.value           = 'custom';
        hl7Dropdown.style.display  = 'none';
        hl7TextInput.style.display = 'block';
        hl7TextInput.value         = hl7Key;

        // Highlight the selected row briefly
        document.querySelectorAll('.hl7-field-row').forEach(r => r.style.background = '');
        const selected = document.querySelector(`.hl7-field-row[data-hl7key="${hl7Key}"]`);
        if (selected) {
            selected.style.background = '#dbeafe';
            setTimeout(() => { selected.style.background = ''; }, 800);
        }

        // Reset the browser loaded flag so it reloads fresh next open
        this._hl7BrowserLoaded = false;

        // The payload browser doesn't carry per-field data types yet, so clear
        // any previously detected composite type and re-check. When hl7DataType
        // is unknown the composite picker stays hidden (graceful degradation).
        const hl7Input = document.getElementById('editHl7Field');
        if (hl7Input) hl7Input.dataset.hl7dtype = '';
        this._checkCompositeMode();
    }

    /**
     * Toggle source input based on source type selection (NO-CODE)
     */
    toggleSourceInput() {
        const sourceType   = document.getElementById('editSourceType').value;
        const hl7Dropdown  = document.getElementById('editHl7FieldDropdown');
        const customInput  = document.getElementById('editHl7Field');
        const staticInput  = document.getElementById('editStaticValue');
        const browser      = document.getElementById('hl7PayloadBrowser');
        const tip          = document.getElementById('editSourceTip');

        // Reset all
        hl7Dropdown.style.display  = 'none';
        customInput.style.display  = 'none';
        staticInput.style.display  = 'none';
        if (browser) browser.style.display = 'none';

        if (sourceType === 'hl7') {
            hl7Dropdown.style.display = 'block';
            if (browser) browser.style.display = 'block';
            if (tip) tip.textContent = '💡 Select from dropdown, type manually, or browse the payload below';
        } else if (sourceType === 'enriched') {
            customInput.style.display  = 'block';
            customInput.placeholder    = 'e.g., ["database_enrichment"].enriched_data.fieldName';
            if (tip) tip.textContent = '💡 Reference enriched data from a previous step';
        } else if (sourceType === 'static_value') {
            staticInput.style.display  = 'block';
            staticInput.focus();
            if (tip) tip.textContent = '💡 This literal value will be written directly to the FHIR path — no HL7 field needed';
        } else {
            // Custom XPath
            customInput.style.display  = 'block';
            customInput.placeholder    = 'Enter custom XPath expression';
            if (browser) browser.style.display = 'block';
            if (tip) tip.textContent = '💡 Type any custom path manually';
        }
    }

    /**
     * Browse variables from previous steps (NO-CODE)
     */
    browseVariables() {
        // Switch to Variables tab in properties panel
        const variablesTab = document.querySelector('.properties-tab[data-tab="variables"]');
        if (variablesTab) {
            variablesTab.click();
            this.builder.dragDropManager.showNotification('Click any XPath in Variables tab to copy it, then return here to paste', 'info');
        } else {
            this.builder.dragDropManager.showNotification('Variables tab not available. Add enrichment steps first.', 'warning');
        }
    }

    /**
     * Switch to Variables tab for browsing
     */
    switchToVariablesTab() {
        const variablesTab = document.querySelector('.properties-tab[data-tab="variables"]');
        if (variablesTab) {
            variablesTab.click();
            this.builder.dragDropManager.showNotification('Click any variable path to copy, then switch back to Form tab to paste', 'info');
        } else {
            this.builder.dragDropManager.showNotification('Add enrichment steps first to see available variables', 'warning');
        }
    }

    /**
     * Toggle source input type for generic field mapping
     */
    toggleGenericSourceInput() {
        const sourceType = document.getElementById('newMappingSourceType').value;
        const dropdown = document.getElementById('newMappingRHSDropdown');
        const textInput = document.getElementById('newMappingRHSText');

        if (sourceType === 'hl7') {
            // Show HL7 dropdown
            dropdown.style.display = 'block';
            textInput.style.display = 'none';
            textInput.placeholder = 'Enter value...';
        } else if (sourceType === 'variable') {
            // Show text input for variable reference
            dropdown.style.display = 'none';
            textInput.style.display = 'block';
            textInput.placeholder = 'e.g., ["step_name"].enriched_data.field';
        } else {
            // Show text input for custom value
            dropdown.style.display = 'none';
            textInput.style.display = 'block';
            textInput.placeholder = 'Enter custom value...';
        }
    }

    /**
     * Show transformation help guide
     */
    showTransformHelp() {
        const helpHTML = `
            <div id="transformHelpModal" class="modal" style="display: flex;">
                <div class="modal-content" style="max-width: 700px;">
                    <div class="modal-header" style="background: #1e3a8a; color: white; border-bottom: 3px solid #ffc0cb;">
                        <h3 style="margin: 0; font-weight: 600; font-size: 1.1rem;">Transformation Guide</h3>
                        <button class="modal-close" style="background: rgba(255,255,255,0.15); border: none; color: white; font-size: 20px; cursor: pointer; width: 28px; height: 28px; border-radius: 3px;" onclick="this.closest('.modal').remove()">×</button>
                    </div>
                    <div class="modal-body" style="padding: 20px; max-height: 60vh; overflow-y: auto;">
                        <h4 style="color: #1e3a8a; margin-top: 0;">Available Transformations</h4>

                        <div style="margin-bottom: 16px; padding: 12px; background: #f0f4ff; border-left: 3px solid #1e3a8a; border-radius: 4px;">
                            <strong style="color: #1e3a8a;">trim</strong>
                            <p style="margin: 4px 0 0 0; color: #64748b; font-size: 0.9rem;">Removes whitespace from both ends of the string.</p>
                            <code style="display: block; margin-top: 4px; padding: 4px 8px; background: white; border-radius: 3px; font-size: 0.85rem;">Example: trim</code>
                        </div>

                        <div style="margin-bottom: 16px; padding: 12px; background: #f0f4ff; border-left: 3px solid #1e3a8a; border-radius: 4px;">
                            <strong style="color: #1e3a8a;">upper</strong>
                            <p style="margin: 4px 0 0 0; color: #64748b; font-size: 0.9rem;">Converts all characters to uppercase.</p>
                            <code style="display: block; margin-top: 4px; padding: 4px 8px; background: white; border-radius: 3px; font-size: 0.85rem;">Example: upper</code>
                        </div>

                        <div style="margin-bottom: 16px; padding: 12px; background: #f0f4ff; border-left: 3px solid #1e3a8a; border-radius: 4px;">
                            <strong style="color: #1e3a8a;">lower</strong>
                            <p style="margin: 4px 0 0 0; color: #64748b; font-size: 0.9rem;">Converts all characters to lowercase.</p>
                            <code style="display: block; margin-top: 4px; padding: 4px 8px; background: white; border-radius: 3px; font-size: 0.85rem;">Example: lower</code>
                        </div>

                        <div style="margin-bottom: 16px; padding: 12px; background: #f0f4ff; border-left: 3px solid #1e3a8a; border-radius: 4px;">
                            <strong style="color: #1e3a8a;">regex:pattern</strong>
                            <p style="margin: 4px 0 0 0; color: #64748b; font-size: 0.9rem;">Extracts text matching a regular expression pattern.</p>
                            <code style="display: block; margin-top: 4px; padding: 4px 8px; background: white; border-radius: 3px; font-size: 0.85rem;">Example: regex:^[0-9]+$ (numbers only)</code>
                            <code style="display: block; margin-top: 4px; padding: 4px 8px; background: white; border-radius: 3px; font-size: 0.85rem;">Example: regex:[A-Z]{2}[0-9]{4} (2 letters + 4 digits)</code>
                        </div>

                        <div style="margin-bottom: 16px; padding: 12px; background: #f0f4ff; border-left: 3px solid #1e3a8a; border-radius: 4px;">
                            <strong style="color: #1e3a8a;">substring:start:end</strong>
                            <p style="margin: 4px 0 0 0; color: #64748b; font-size: 0.9rem;">Extracts characters from position start to end (zero-based index).</p>
                            <code style="display: block; margin-top: 4px; padding: 4px 8px; background: white; border-radius: 3px; font-size: 0.85rem;">Example: substring:0:10 (first 10 characters)</code>
                            <code style="display: block; margin-top: 4px; padding: 4px 8px; background: white; border-radius: 3px; font-size: 0.85rem;">Example: substring:5:15 (characters 5 through 15)</code>
                        </div>

                        <div style="margin-bottom: 16px; padding: 12px; background: #f0f4ff; border-left: 3px solid #1e3a8a; border-radius: 4px;">
                            <strong style="color: #1e3a8a;">replace:old:new</strong>
                            <p style="margin: 4px 0 0 0; color: #64748b; font-size: 0.9rem;">Replaces all occurrences of "old" text with "new" text.</p>
                            <code style="display: block; margin-top: 4px; padding: 4px 8px; background: white; border-radius: 3px; font-size: 0.85rem;">Example: replace:-:/ (replace dashes with slashes)</code>
                            <code style="display: block; margin-top: 4px; padding: 4px 8px; background: white; border-radius: 3px; font-size: 0.85rem;">Example: replace:Dr.:Doctor (expand abbreviation)</code>
                        </div>

                        <div style="margin-bottom: 16px; padding: 12px; background: #fdf2f8; border-left: 3px solid #ffc0cb; border-radius: 4px;">
                            <strong style="color: #831843;">Chaining Transformations</strong>
                            <p style="margin: 4px 0 0 0; color: #64748b; font-size: 0.9rem;">Combine multiple transformations by separating them with commas. They are applied left to right.</p>
                            <code style="display: block; margin-top: 4px; padding: 4px 8px; background: white; border-radius: 3px; font-size: 0.85rem;">Example: trim, upper</code>
                            <code style="display: block; margin-top: 4px; padding: 4px 8px; background: white; border-radius: 3px; font-size: 0.85rem;">Example: trim, substring:0:10, upper</code>
                            <code style="display: block; margin-top: 4px; padding: 4px 8px; background: white; border-radius: 3px; font-size: 0.85rem;">Example: replace:-:/, regex:^[0-9/]+$</code>
                        </div>
                    </div>
                    <div class="modal-footer" style="border-top: 1px solid #e2e8f0; padding: 16px 20px; display: flex; justify-content: flex-end;">
                        <button onclick="this.closest('.modal').remove()" style="background: #1e3a8a; color: white; border: none; padding: 8px 20px; border-radius: 4px; cursor: pointer; font-weight: 500;">
                            Got it!
                        </button>
                    </div>
                </div>
            </div>
        `;
        document.body.insertAdjacentHTML('beforeend', helpHTML);
    }

    /**
     * Select FHIR path from dropdown (NO-CODE)
     */
    // ── FHIR path combobox ─────────────────────────────────────────────────────

    /**
     * Builds grouped option data from _displayedTemplateMappings.
     * Returns [{resourceType, paths:[fullPath,...]}] sorted alphabetically.
     * Pure data — no DOM touch.
     */
    _buildFhirPathOptions(extraMappings = []) {
        const src = [...(this._displayedTemplateMappings || []), ...extraMappings];
        const seen   = new Set();
        const groups = new Map(); // resourceType → Set<fullPath>

        src.forEach(m => {
            const rt  = m.resourceType || m.fhirResourceType || '';
            const raw = m.fhirPath || m.targetPath || m.targetField || '';
            if (!rt || !raw) return;

            // Exclude bare FHIR polymorphic type placeholders produced by the
            // Z-segment logic gap (valueDecimal, valueString, valueCode, etc.).
            // A bare "valueFoo" with no dots/brackets is never a valid FHIR
            // element path on a resource root.
            if (/^value[A-Z][a-z]+$/.test(raw)) return;

            // raw may be "name[0].family" or "Patient.name[0].family" — normalise
            const full = raw.startsWith(rt + '.') ? raw : `${rt}.${raw}`;
            if (seen.has(full)) return;
            seen.add(full);

            if (!groups.has(rt)) groups.set(rt, []);
            groups.get(rt).push(full);
        });

        return [...groups.entries()]
            .sort(([a], [b]) => a.localeCompare(b))
            .map(([resourceType, paths]) => ({
                resourceType,
                paths: paths.sort(),
            }));
    }

    /**
     * Fetches the OOB standard template for this message type and merges its
     * FHIR paths into _fhirPathGroups.  Runs async — the dropdown is already
     * usable with interface mappings while this completes in the background.
     */
    async _enrichFhirDropdownWithOOB() {
        const messageType = this.currentStep?.config?.message_type
                         || this.builder.pipeline?.messageType
                         || 'ADT^A01';
        try {
            const resp = await window.pipelineAPI.getStandardTemplateMappings(messageType);
            if (!resp.success || !resp.data?.mappings?.length) return;

            this._fhirPathGroups = this._buildFhirPathOptions(resp.data.mappings);

            // If the suggestions panel is currently open, refresh it live
            const box = document.getElementById('editFhirSuggestions');
            if (box && box.style.display !== 'none') {
                const inp = document.getElementById('editFhirPath');
                this._filterFhirDropdown(inp?.value || '');
            }
        } catch (e) {
            // Silent — interface-only paths remain available
        }
    }

    /** Renders suggestion HTML for the given groups + optional search query. */
    _renderFhirSuggestions(groups, query) {
        const q = (query || '').trim().toLowerCase();

        if (!groups.length) {
            return `<div style="padding:0.75rem; text-align:center; color:#94a3b8; font-size:0.82rem;">
                No mappings loaded — open the mapping panel first
            </div>`;
        }

        const esc    = s => String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
        const hilite = (text) => {
            if (!q) return esc(text);
            const re = new RegExp(`(${q.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')})`, 'gi');
            return esc(text).replace(re, '<mark style="background:#fef08a;border-radius:2px;padding:0 1px;">$1</mark>');
        };

        let html = '';
        let totalShown = 0;

        groups.forEach(({ resourceType, paths }) => {
            const filtered = q ? paths.filter(p => p.toLowerCase().includes(q)) : paths;
            if (!filtered.length) return;
            totalShown += filtered.length;

            html += `<div>
                <div style="padding:0.3rem 0.75rem; background:#f8fafc; border-bottom:1px solid #f1f5f9;
                            font-size:0.72rem; font-weight:700; color:#475569;
                            text-transform:uppercase; letter-spacing:0.5px; position:sticky; top:0; z-index:1;">
                    ${esc(resourceType)}
                </div>
                ${filtered.map(path => `
                    <div class="fhir-suggestion-item" data-value="${esc(path)}"
                        style="padding:0.38rem 0.75rem 0.38rem 1.25rem; cursor:pointer;
                               font-size:0.82rem; color:#1e293b; border-bottom:1px solid #f8fafc;"
                        onmouseover="this.style.background='#eff6ff'"
                        onmouseout="this.style.background=''"
                        onmousedown="window.propertiesPanel._selectFhirPath('${esc(path)}')">
                        ${hilite(path)}
                    </div>`).join('')}
            </div>`;
        });

        if (totalShown === 0 && q) {
            html = `<div style="padding:0.75rem; font-size:0.82rem; color:#64748b; text-align:center;">
                No match — <strong>"${esc(q)}"</strong> will be used as a custom path
            </div>`;
        }

        return html;
    }

    _showFhirDropdown() {
        const box = document.getElementById('editFhirSuggestions');
        const inp = document.getElementById('editFhirPath');
        if (!box) return;
        box.innerHTML = this._renderFhirSuggestions(
            this._fhirPathGroups || [],
            inp?.value || ''
        );
        box.style.display = 'block';
    }

    _filterFhirDropdown(query) {
        const box = document.getElementById('editFhirSuggestions');
        if (!box) return;
        if (box.style.display === 'none') box.style.display = 'block';
        box.innerHTML = this._renderFhirSuggestions(
            this._fhirPathGroups || [],
            query
        );
    }

    _hideFhirDropdown() {
        const box = document.getElementById('editFhirSuggestions');
        if (box) box.style.display = 'none';
    }

    _selectFhirPath(path) {
        const inp = document.getElementById('editFhirPath');
        if (inp) inp.value = path;
        this._hideFhirDropdown();
    }

    /** Arrow-key + Enter navigation inside the FHIR suggestions. */
    _fhirDropdownKeydown(e) {
        const box   = document.getElementById('editFhirSuggestions');
        const items = box ? [...box.querySelectorAll('.fhir-suggestion-item')] : [];
        const cur   = box ? box.querySelector('.fhir-suggestion-item[data-focused]') : null;

        if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
            e.preventDefault();
            if (!items.length) { this._showFhirDropdown(); return; }
            let idx = cur ? items.indexOf(cur) : -1;
            if (cur) { cur.style.background = ''; delete cur.dataset.focused; }
            idx = e.key === 'ArrowDown'
                ? Math.min(idx + 1, items.length - 1)
                : Math.max(idx - 1, 0);
            items[idx].style.background = '#eff6ff';
            items[idx].dataset.focused  = '1';
            items[idx].scrollIntoView({ block: 'nearest' });
        } else if (e.key === 'Enter') {
            if (cur) { this._selectFhirPath(cur.dataset.value); e.preventDefault(); }
        } else if (e.key === 'Escape') {
            this._hideFhirDropdown();
        }
    }

    /**
     * Save edited mapping
     */
    saveEditedMapping(index) {
        const sourceType = document.getElementById('editSourceType')?.value || 'hl7';
        const fhirPath   = document.getElementById('editFhirPath').value.trim();

        if (!fhirPath) {
            this.builder.dragDropManager.showNotification('FHIR Target Path is required', 'error');
            return;
        }

        let mappingObject;

        if (sourceType === 'static_value') {
            // ── Static value mapping ────────────────────────────────────────
            const staticValue = document.getElementById('editStaticValue')?.value.trim() || '';
            if (!staticValue) {
                this.builder.dragDropManager.showNotification('Static Value is required', 'error');
                return;
            }
            mappingObject = {
                sourcePath:    '',
                hl7Field:      '',
                fhirPath,
                targetPath:    fhirPath,
                staticValue,
                defaultValue:  staticValue,
                transformType: 'static_value',
                dataType:      '',
            };
        } else {
            // ── HL7 / enriched / custom source mapping ───────────────────────
            let hl7Field = document.getElementById('editHl7Field').value.trim();
            if (!hl7Field) {
                const dropdown = document.getElementById('editHl7FieldDropdown');
                if (dropdown && dropdown.value) hl7Field = dropdown.value;
            }
            if (!hl7Field) {
                this.builder.dragDropManager.showNotification('HL7 Field is required', 'error');
                return;
            }
            const transformType = document.getElementById('editTransformType')?.value || '';
            const dataType      = document.getElementById('editDataType')?.value || '';
            mappingObject = {
                hl7Field,
                sourcePath:    hl7Field,
                fhirPath,
                targetPath:    fhirPath,
                transformType,
                dataType,
            };
        }

        // Close modal immediately so the user sees feedback
        document.getElementById('editMappingModal').remove();

        if (this._isInterfaceRefMode()) {
            // ── Interface-ref mode: persist via delta endpoint ──────────────────
            // Work against _displayedTemplateMappings (the live fetched list).
            // The backend diffs the full list against OOB and stores only the delta.
            if (!this._displayedTemplateMappings) this._displayedTemplateMappings = [];

            if (index === undefined) {
                this._displayedTemplateMappings.push(mappingObject);
            } else {
                // Merge: keep backend fields (id, confidence, isRequired) that the
                // edit modal may not have touched.
                this._displayedTemplateMappings[index] = Object.assign(
                    {}, this._displayedTemplateMappings[index], mappingObject
                );
            }

            const atomicMappings = this._toAtomicMappings(this._displayedTemplateMappings);
            this._saveMappingDelta(atomicMappings)
                .then(result => {
                    const label = index === undefined ? 'Mapping added' : 'Mapping updated';
                    if (result.isPureOOB) {
                        this.builder.dragDropManager.showNotification(
                            `${label} — but your changes match the standard template, so no custom override was stored. If you intended to customise this mapping, try changing a value that differs from the OOB default.`,
                            'warning'
                        );
                    } else {
                        const detail = ` (${result.overrideCount} override${result.overrideCount !== 1 ? 's' : ''})`;
                        this.builder.dragDropManager.showNotification(label + detail, 'success');
                    }
                    // Re-render preserving the mapping table scroll position
                    this._refreshMappingPanel();
                })
                .catch(err => {
                    this.builder.dragDropManager.showNotification('Save failed: ' + err.message, 'error');
                    console.error('[Field Mapping] Delta save failed:', err);
                });
        } else {
            // ── Embedded / config mode: write directly to step config ───────────
            if (index === undefined) {
                this.currentStep.config.mappings.push(mappingObject);
                this.builder.dragDropManager.showNotification('Mapping added', 'success');
            } else {
                this.currentStep.config.mappings[index] = mappingObject;
                this.builder.dragDropManager.showNotification('Mapping updated', 'success');
            }

            this.builder.updateStep(this.currentStep);
            this.builder.savePipeline().then(() => {
                console.log('[Field Mapping] Pipeline auto-saved after mapping change');
            }).catch(err => {
                console.error('[Field Mapping] Auto-save failed:', err);
            });

            this._refreshMappingPanel();
            this.builder.markAsUnsaved();
        }
    }

    /**
     * Delete a mapping
     */
    async deleteMapping(index) {
        const confirmed = await this.builder.dragDropManager.showConfirmDialog(
            'Are you sure you want to delete this mapping?',
            {
                title: 'Delete Mapping',
                confirmText: 'Delete',
                cancelText: 'Cancel',
                type: 'danger'
            }
        );

        if (!confirmed) return;

        if (this._isInterfaceRefMode()) {
            // ── Interface-ref mode: remove from live list, persist via delta ───
            if (!this._displayedTemplateMappings || index >= this._displayedTemplateMappings.length) return;

            // Keep a copy in case we need to roll back on failure
            const removed = this._displayedTemplateMappings.splice(index, 1)[0];
            const atomicMappings = this._toAtomicMappings(this._displayedTemplateMappings);

            this._saveMappingDelta(atomicMappings)
                .then(result => {
                    const detail = result.isPureOOB ? ' (back to pure OOB)' : ` (${result.overrideCount} override${result.overrideCount !== 1 ? 's' : ''} remain)`;
                    this.builder.dragDropManager.showNotification('Mapping deleted' + detail, 'success');
                    this.showStepProperties(this.currentStep);
                })
                .catch(err => {
                    // Restore the removed item so the UI stays consistent
                    this._displayedTemplateMappings.splice(index, 0, removed);
                    this.builder.dragDropManager.showNotification('Delete failed: ' + err.message, 'error');
                    console.error('[Field Mapping] Delta delete failed:', err);
                });
        } else {
            // ── Embedded / config mode: splice from step config ─────────────
            if (!this.currentStep?.config?.mappings) return;

            this.currentStep.config.mappings.splice(index, 1);

            this.builder.updateStep(this.currentStep);
            this.builder.savePipeline().then(() => {
                console.log('[Field Mapping] Pipeline auto-saved after mapping deletion');
            }).catch(err => {
                console.error('[Field Mapping] Auto-save failed:', err);
            });

            this.showStepProperties(this.currentStep);
            this.builder.markAsUnsaved();
            this.builder.dragDropManager.showNotification('Mapping deleted', 'success');
        }
    }

    // ── Delta helpers ──────────────────────────────────────────────────────────

    /**
     * Returns true when the current step resolves its mappings from the interface
     * (interface_ref mode).  In this mode edits must go to the delta endpoint,
     * not to config.mappings.
     */
    _isInterfaceRefMode() {
        return !!(this.currentStep?.config?.interface_id ||
                  this.builder.pipeline?.interfaceId);
    }

    /**
     * Converts the display-format mapping objects (from _displayedTemplateMappings)
     * to the AtomicMapping shape expected by the delta endpoint.
     *
     * Display objects may carry hl7Field/fhirPath keys (from the edit modal) or
     * sourcePath/targetPath keys (as returned by the backend).  Both are handled.
     *
     * FHIR paths from the edit modal include the resource type prefix
     * ("Patient.name[0].family").  AtomicMapping splits that into:
     *   resourceType = "Patient"
     *   targetPath   = "name[0].family"
     */
    _toAtomicMappings(displayMappings) {
        return displayMappings.map(m => {
            const hl7Path    = m.sourcePath  || m.hl7Field    || m.sourceField || '';
            const rawFhir    = m.fhirPath    || m.targetPath  || m.targetField || '';
            let   resourceType = m.resourceType || m.fhirResourceType || '';
            let   targetPath   = rawFhir;

            // Split "Patient.name[0].family" → resourceType + targetPath only when
            // resourceType is not already known.
            if (!resourceType && rawFhir) {
                const match = rawFhir.match(/^([A-Z][A-Za-z]+)\.(.+)$/);
                if (match) { resourceType = match[1]; targetPath = match[2]; }
            }

            const isStatic = (m.transformType === 'static_value') ||
                             (m.staticValue && !hl7Path);
            return {
                id:               m.id            || '',
                sourcePath:       isStatic ? '' : hl7Path,
                targetPath:       targetPath,
                resourceType:     resourceType,
                fhirResourceType: resourceType,
                transformType:    isStatic ? 'static_value' : (m.transformType || ''),
                defaultValue:     isStatic ? (m.staticValue || m.defaultValue || '') : '',
                isRequired:       m.isRequired     || false,
                confidence:       m.confidence     || 0,
            };
        });
    }

    /**
     * Sends the full current mapping list to the delta endpoint.
     * The backend diffs against the OOB template and stores only what changed.
     * Returns the response payload (includes overrideCount, isPureOOB).
     */
    async _saveMappingDelta(atomicMappings) {
        const interfaceId  = this.currentStep?.config?.interface_id || this.builder.pipeline?.interfaceId;
        const messageType  = this.currentStep?.config?.message_type || this.builder.pipeline?.messageType || 'ADT^A01';
        const resp = await fetch(
            `/api/fhir/interfaces/${encodeURIComponent(interfaceId)}/mapping-delta/${encodeURIComponent(messageType)}`,
            {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                credentials: 'include',
                body: JSON.stringify({ atomicMappings }),
            }
        );
        const data = await resp.json();
        if (!data.success) throw new Error(data.error || 'Failed to save mapping delta');
        return data;
    }

    /**
     * Handle mapping file upload
     */
    async handleMappingFileUpload(file, step) {
        if (!file) return;

        try {
            const text = await file.text();
            const data = JSON.parse(text);

            // Validate structure
            if (data.mappings && Array.isArray(data.mappings)) {
                step.config = data;
                this.showStepProperties(step); // Refresh UI
                this.builder.dragDropManager.showNotification(`Loaded ${data.mappings.length} mappings`, 'success');
            } else {
                throw new Error('Invalid mapping file structure. Expected { mappings: [...] }');
            }
        } catch (error) {
            this.builder.dragDropManager.showNotification(`Upload failed: ${error.message}`, 'error');
        }
    }

    /**
     * Export mappings as JSON file
     */
    exportMappingsAsJSON(step) {
        const blob = new Blob([JSON.stringify(step.config, null, 2)], { type: 'application/json' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `${step.stepName.replace(/\s+/g, '_')}_mappings.json`;
        a.click();
        URL.revokeObjectURL(url);
        this.builder.dragDropManager.showNotification('Mappings exported', 'success');
    }

    /**
     * Refresh mappings for the HL7→FHIR step.
     * When interface_id is present: fetches the live resolved mapping (includes Z-segments,
     * AI additions, delta overrides — whatever the runtime would use).
     * Falls back to the OOB standard template when there is no interface context.
     */
    async loadStandardTemplateMappings(step) {
        try {
            const interfaceId = step.config?.interface_id || this.builder.pipeline?.interfaceId;
            const messageType = step.config?.message_type || this.builder.pipeline?.messageType || 'ADT^A01';
            const tableContainer = document.getElementById('mappingTableContainer');
            const statusText    = document.getElementById('mappingStatusText');

            this.builder.dragDropManager.showNotification(`Refreshing mappings for ${messageType}…`, 'info');

            let mappings = [];
            let statusHtml = '';

            if (interfaceId) {
                // ── Live resolved mapping (the source of truth) ──
                const resp = await fetch(
                    `/api/fhir/interfaces/${encodeURIComponent(interfaceId)}/resolved-mappings?messageType=${encodeURIComponent(messageType)}`,
                    { credentials: 'include' }
                );
                const data = await resp.json();
                if (!data.success) throw new Error(data.error || 'resolved-mappings failed');
                mappings = data.atomicMappings || [];
                const modeLabels = { oob: '🔗 Tracking OOB template', delta: '⚡ OOB + custom additions', custom: '✏️ Fully custom' };
                statusHtml = `<strong>${mappings.length}</strong> mappings
                    <span style="margin-left:6px;font-size:11px;padding:2px 7px;border-radius:10px;background:#e0f2fe;color:#0369a1;font-weight:600;">
                        ${modeLabels[data.mappingMode] || data.mappingMode}
                    </span>`;
            } else {
                // ── OOB fallback ──
                const response = await window.pipelineAPI.getStandardTemplateMappings(messageType);
                if (!response.success) throw new Error(response.error || 'Failed to fetch template');
                const { template, mappings: tplMappings } = response.data;
                mappings = tplMappings || [];
                statusHtml = `<strong>${mappings.length}</strong> mappings
                    <span style="color:#059669;font-weight:500;">(OOB: ${template?.name || 'standard'})</span>`;
            }

            if (!mappings.length) {
                this.builder.dragDropManager.showNotification(`No mappings found for ${messageType}`, 'warning');
                return;
            }

            if (tableContainer) tableContainer.innerHTML = this.renderMappingTable(mappings);
            if (statusText)    statusText.innerHTML = statusHtml;

            this.builder.dragDropManager.showNotification(`Loaded ${mappings.length} mappings`, 'success');
            this.builder.markAsUnsaved();

        } catch (error) {
            console.error('Error refreshing mappings:', error);
            this.builder.dragDropManager.showNotification(`Failed to load mappings: ${error.message}`, 'error');
        }
    }

    /**
     * Auto-load mappings for display — always called when the step is opened.
     * When interface_id is present: fetches the live resolved mapping from the backend
     * (exactly what the runtime 5-step chain would use, including Z-segments and AI additions).
     * Falls back to the OOB standard template when no interface_id is set.
     */
    async autoLoadTemplateMappings(step, container) {
        try {
            const interfaceId = container?.dataset?.interfaceId || step.config?.interface_id || this.builder.pipeline?.interfaceId;
            const messageType = container?.dataset?.messageType || step.config?.message_type || this.builder.pipeline?.messageType || 'ADT^A01';

            let mappings = [];
            let sourceLabel = '';
            let mappingMode = container?.dataset?.mappingMode || step.config?.mapping_mode || 'oob';

            if (interfaceId) {
                // ── Primary path: resolved-mappings endpoint (live, includes all customisations) ──
                console.log(`🔄 Fetching resolved mappings for interface ${interfaceId} / ${messageType}…`);
                const resp = await fetch(
                    `/api/fhir/interfaces/${encodeURIComponent(interfaceId)}/resolved-mappings?messageType=${encodeURIComponent(messageType)}`,
                    { credentials: 'include' }
                );
                const data = await resp.json();
                if (!data.success) throw new Error(data.error || 'resolved-mappings failed');
                mappings = data.atomicMappings || [];
                mappingMode = data.mappingMode || mappingMode;
                const modeLabels = { oob: '🔗 Tracking OOB template', delta: '⚡ OOB + custom additions', custom: '✏️ Fully custom' };
                sourceLabel = `<span style="margin-left:6px;font-size:11px;padding:2px 7px;border-radius:10px;background:#e0f2fe;color:#0369a1;font-weight:600;">${modeLabels[mappingMode] || mappingMode}</span>`;
            } else {
                // ── Fallback: OOB standard template (no interface context) ──
                console.log(`🔄 Auto-fetching OOB template for ${messageType}…`);
                const response = await window.pipelineAPI.getStandardTemplateMappings(messageType);
                if (!response.success) throw new Error(response.error || 'Failed to fetch template');
                const { template, mappings: tplMappings } = response.data;
                mappings = tplMappings || [];
                sourceLabel = `<span style="color:#059669;font-weight:500;">(OOB template: ${template?.name || 'standard'})</span>`;
            }

            if (!mappings || mappings.length === 0) {
                container.innerHTML = `
                    <div style="padding: 2rem; text-align: center; color: #64748b;">
                        <i class="fas fa-info-circle fa-2x" style="color: #f59e0b;"></i>
                        <p style="margin-top: 1rem;">No mappings found for ${messageType}</p>
                        <p style="font-size: 0.85rem;">Complete the wizard to configure mappings, or add them manually below.</p>
                    </div>
                `;
                return;
            }

            // Update the status bar
            const statusText = document.getElementById('mappingStatusText');
            if (statusText) {
                statusText.innerHTML = `<strong>${mappings.length}</strong> mappings ${sourceLabel}`;
            }

            // Render the mappings table
            container.innerHTML = this.renderMappingTable(mappings);
            this._displayedTemplateMappings = mappings;

            console.log(`✅ Loaded ${mappings.length} mappings (mode: ${mappingMode})`);

        } catch (error) {
            console.error('Error loading mappings:', error);
            container.innerHTML = `
                <div style="padding: 2rem; text-align: center; color: #ef4444;">
                    <i class="fas fa-exclamation-triangle fa-2x"></i>
                    <p style="margin-top: 1rem;">Failed to load template: ${error.message}</p>
                    <button id="retryLoadTemplateBtn" class="btn btn-secondary" style="margin-top: 0.5rem;">
                        <i class="fas fa-redo"></i> Retry
                    </button>
                </div>
            `;
            // Add retry handler
            const retryBtn = container.querySelector('#retryLoadTemplateBtn');
            if (retryBtn) {
                retryBtn.addEventListener('click', () => this.autoLoadTemplateMappings(step, container));
            }
        }
    }

    /**
     * Load FHIR resource preview (confidence + field counts) for the Resources tab
     */
    async loadResourcePreview(step, form) {
        const cardsArea = form.querySelector('#resourceCardsArea');
        if (!cardsArea) return;

        const messageType = this.builder.pipeline?.messageType || step.config?.message_type || 'ADT^A01';

        cardsArea.innerHTML = `<div style="padding: 1.5rem; text-align: center; color: #64748b;">
            <i class="fas fa-spinner fa-spin fa-lg"></i>
            <p style="margin-top: 0.5rem; font-size: 0.875rem;">Loading resource info for ${messageType}…</p>
        </div>`;

        try {
            const resp = await fetch(`/api/fhir/template/preview?messageType=${encodeURIComponent(messageType)}`, {
                credentials: 'include'
            });
            const data = await resp.json();
            if (!data.success) throw new Error(data.error || 'Unknown error');

            // Current selection stored in step config
            const selectedResources = Array.isArray(step.config?.selected_resources)
                ? step.config.selected_resources
                : null; // null = all selected (default)

            const confColor = (avg) => {
                if (avg >= 0.9) return '#059669';
                if (avg >= 0.75) return '#d97706';
                return '#dc2626';
            };
            const confLabel = (avg) => {
                if (avg >= 0.9) return 'High';
                if (avg >= 0.75) return 'Medium';
                return 'Low';
            };

            cardsArea.innerHTML = data.resources.map(r => {
                const isChecked = selectedResources === null || selectedResources.includes(r.name);
                const pct = Math.round(r.avg_confidence * 100);
                const color = confColor(r.avg_confidence);
                return `
                <div class="resource-card" style="
                    display: flex; align-items: flex-start; gap: 0.75rem;
                    padding: 0.875rem 1rem;
                    margin-bottom: 0.5rem;
                    border: 1px solid ${isChecked ? '#bfdbfe' : '#e5e7eb'};
                    border-radius: 8px;
                    background: ${isChecked ? '#f0f9ff' : '#f8fafc'};
                    transition: all 0.2s ease;
                ">
                    <input type="checkbox" class="resource-checkbox" data-resource="${r.name}"
                        ${isChecked ? 'checked' : ''}
                        style="margin-top: 3px; width: 16px; height: 16px; cursor: pointer; accent-color: #1e3a8a;">
                    <div style="flex: 1; min-width: 0;">
                        <div style="display: flex; align-items: center; gap: 0.5rem; flex-wrap: wrap; margin-bottom: 0.35rem;">
                            <span style="font-weight: 600; font-size: 0.9rem; color: #1e293b;">${r.name}</span>
                            ${r.optional ? '<span style="font-size: 0.7rem; background: #fef3c7; color: #92400e; padding: 1px 6px; border-radius: 10px;">optional</span>' : '<span style="font-size: 0.7rem; background: #dcfce7; color: #166534; padding: 1px 6px; border-radius: 10px;">core</span>'}
                            <span style="font-size: 0.75rem; color: #64748b; margin-left: auto;">${r.field_count} fields · ${r.required_fields} required</span>
                        </div>
                        <!-- Confidence bar -->
                        <div style="display: flex; align-items: center; gap: 0.5rem;">
                            <div style="flex: 1; height: 6px; background: #e5e7eb; border-radius: 3px; overflow: hidden;">
                                <div style="width: ${pct}%; height: 100%; background: ${color}; border-radius: 3px; transition: width 0.4s ease;"></div>
                            </div>
                            <span style="font-size: 0.75rem; font-weight: 600; color: ${color}; white-space: nowrap;">${pct}% ${confLabel(r.avg_confidence)}</span>
                        </div>
                    </div>
                </div>`;
            }).join('');

            // Wire up checkboxes → update step.config.selected_resources
            cardsArea.querySelectorAll('.resource-checkbox').forEach(cb => {
                cb.addEventListener('change', () => {
                    const checked = [...cardsArea.querySelectorAll('.resource-checkbox:checked')]
                        .map(el => el.dataset.resource);
                    // If all resources selected, store empty array (means "all")
                    const allResources = data.resources.map(r => r.name);
                    if (checked.length === allResources.length) {
                        delete step.config.selected_resources;
                    } else {
                        if (!step.config) step.config = {};
                        step.config.selected_resources = checked;
                    }
                    // Update card styles
                    cardsArea.querySelectorAll('.resource-card').forEach(card => {
                        const cardCb = card.querySelector('.resource-checkbox');
                        card.style.borderColor = cardCb.checked ? '#bfdbfe' : '#e5e7eb';
                        card.style.background = cardCb.checked ? '#f0f9ff' : '#f8fafc';
                    });
                });
            });

        } catch (err) {
            cardsArea.innerHTML = `<div style="padding: 1.5rem; text-align: center; color: #dc2626;">
                <i class="fas fa-exclamation-circle fa-lg"></i>
                <p style="margin-top: 0.5rem; font-size: 0.875rem;">Failed to load resource info: ${err.message}</p>
            </div>`;
        }
    }

    /**
     * Add step to pipeline (from preview mode)
     */
    addStepToPipeline(step) {
        try {
            // Gather form data
            const configuredStep = this.collectFormData(step);

            // Add step to pipeline
            this.builder.addStep(configuredStep);

            // Show success
            this.builder.dragDropManager.showNotification('Step added to pipeline', 'success');

            // Mark as unsaved
            this.builder.markAsUnsaved();

            // Auto-save the pipeline immediately
            console.log('[PropertiesPanel] Auto-saving pipeline after step add...');
            this.builder.savePipeline().then(() => {
                console.log('[PropertiesPanel] Pipeline auto-saved successfully');
                this._silentRefreshTestOutput();
            }).catch(err => {
                console.error('[PropertiesPanel] Auto-save failed:', err);
                this.builder.dragDropManager.showNotification('Warning: Step added but auto-save failed. Please click Save Pipeline manually.', 'warning');
            });

            this.closeModal();

        } catch (error) {
            console.error('Failed to add step to pipeline:', error);
            this.builder.dragDropManager.showNotification(error.message, 'error');
        }
    }

    /**
     * Save properties of existing step
     */
    saveStepProperties(step) {
        try {
            // Gather form data
            const updatedStep = this.collectFormData(step);

            // Update step in pipeline
            this.builder.updateStep(updatedStep);

            // Update node visual
            this.builder.stepNodeManager.updateNode(step.id, updatedStep);

            // Show success
            this.builder.dragDropManager.showNotification('Step updated', 'success');

            // Mark as unsaved
            this.builder.markAsUnsaved();

            // Auto-save the pipeline immediately
            console.log('[PropertiesPanel] Auto-saving pipeline after step add...');
            this.builder.savePipeline().then(() => {
                console.log('[PropertiesPanel] Pipeline auto-saved successfully');
                this._silentRefreshTestOutput();
            }).catch(err => {
                console.error('[PropertiesPanel] Auto-save failed:', err);
                this.builder.dragDropManager.showNotification('Warning: Step added but auto-save failed. Please click Save Pipeline manually.', 'warning');
            });

            this.closeModal();

        } catch (error) {
            console.error('Failed to save step:', error);
            this.builder.dragDropManager.showNotification(error.message, 'error');
        }
    }

    /**
     * Save step (used by ScriptEnrichmentEditor)
     */
    saveStep(step, isPreview = false) {
        if (isPreview) {
            this.addStepToPipeline(step);
        } else {
            this.saveStepProperties(step);
        }
    }

    /**
     * Close step properties modal
     */
    closeModal() {
        const modal = document.getElementById('stepPropertiesModal');
        if (modal) {
            modal.style.display = 'none';
        }
        this.currentStep = null;
        this.ifThenElseBuilder = null; // Clean up builder instance
    }

    /**
     * Collect form data
     */
    collectFormData(step) {
        // Get form from modal content (not this.container which is the right panel)
        const modalContent = document.getElementById('formTabContent') || document.getElementById('stepPropertiesContent');
        const form = modalContent?.querySelector('.properties-form') ||
                     modalContent?.querySelector('.validation-builder') ||
                     modalContent;

        if (!form) {
            throw new Error('Properties form not found');
        }

        console.log('[PropertiesPanel] Collecting data from form:', form.className);
        console.log('[PropertiesPanel] Step before collection:', {
            stepName: step.stepName,
            stepType: step.stepType
        });

        // Basic properties - only update if form fields exist and have values
        const stepNameField = form.querySelector('#stepName');
        if (stepNameField && stepNameField.value) {
            step.stepName = stepNameField.value;
        }

        const descField = form.querySelector('#stepDescription');
        if (descField) {
            step.description = descField.value || '';
        }

        // Icon is automatically assigned based on step type - no manual override needed
        // The VisualStep constructor will handle icon assignment via getIconForType()

        // Execution properties - only update if form fields exist
        const seqField = form.querySelector('#stepSequence');
        if (seqField && seqField.value) {
            step.sequence = parseInt(seqField.value);
        }

        const timeoutField = form.querySelector('#stepTimeout');
        if (timeoutField && timeoutField.value) {
            step.timeoutMs = parseInt(timeoutField.value);
        }

        const errorStrategyField = form.querySelector('#stepErrorStrategy');
        if (errorStrategyField && errorStrategyField.value) {
            step.onErrorStrategy = errorStrategyField.value;
        }

        const requiredField = form.querySelector('#stepRequired');
        if (requiredField) {
            step.required = requiredField.checked;
        }

        const enabledField = form.querySelector('#stepEnabled');
        if (enabledField) {
            step.enabled = enabledField.checked;
        }

        console.log('[PropertiesPanel] Step after collection:', {
            stepName: step.stepName,
            stepType: step.stepType,
            sequence: step.sequence,
            enabled: step.enabled
        });

        // Configuration - check for validation rules hidden field
        const validationRulesInput = form.querySelector('#validationRules');
        if (validationRulesInput && validationRulesInput.value) {
            try {
                step.config = step.config || {};
                step.config.rules = JSON.parse(validationRulesInput.value);
                console.log('[PropertiesPanel] ✅ Saved to step.config.rules:', step.config.rules.length, 'rules');
                console.log('[PropertiesPanel] 🔍 EXACT RULES BEING SAVED:', JSON.stringify(step.config.rules, null, 2));
            } catch (error) {
                console.error('[PropertiesPanel] Failed to parse validation rules:', error);
            }
        }

        // Collect metadata from MetadataBuilder component
        const metadataBuilderContainers = form.querySelectorAll('.metadata-builder-container');
        metadataBuilderContainers.forEach(container => {
            const builder = container._metadataBuilderInstance;
            if (builder) {
                step.config = step.config || {};
                const fieldKey = container.dataset.fieldKey;
                const metadata = builder.getMetadata();
                step.config[fieldKey] = metadata;
                console.log('[PropertiesPanel] ✅ Saved metadata to step.config.' + fieldKey + ':', metadata);
            }
        });

        // Collect headers from HeaderBuilder component
        const headerBuilderContainers = form.querySelectorAll('.header-builder-container');
        headerBuilderContainers.forEach(container => {
            const builder = container._headerBuilderInstance;
            if (builder) {
                step.config = step.config || {};
                const fieldKey = container.dataset.fieldKey;
                const headers = builder.getHeaders();
                step.config[fieldKey] = headers;
                console.log('[PropertiesPanel] ✅ Saved headers to step.config.' + fieldKey + ':', headers);
            }
        });

        // Collect query params from QueryParamBuilder component
        // FIX: Search in the entire modal content, not just the form, because the container might be re-rendered
        const modal = document.getElementById('stepPropertiesModal');
        const searchRoot = modal || form;
        console.log('[PropertiesPanel] 🔍 Searching for query-param-builder-container in:', searchRoot.id || searchRoot.className);
        const queryParamBuilderContainers = searchRoot.querySelectorAll('.query-param-builder-container');
        console.log('[PropertiesPanel] 🔍 Found', queryParamBuilderContainers.length, 'query-param-builder-container(s) during collection');

        // CRITICAL FIX: If no containers found in DOM, use the globally stored builder instance
        if (queryParamBuilderContainers.length === 0 && this._activeQueryParamBuilder) {
            console.log('[PropertiesPanel] 🔧 FALLBACK: Using globally stored QueryParamBuilder instance');
            step.config = step.config || {};
            const queryParams = this._activeQueryParamBuilder.getParams();
            const fieldKey = this._activeQueryParamFieldKey || 'queryParamsBuilder';

            console.log(`[PropertiesPanel] 🔑 getParams() returned from global builder:`, queryParams);

            step.config[fieldKey] = queryParams;

            // For database enrichment, also store as 'queryParams' for backend compatibility
            if (fieldKey === 'queryParamsBuilder') {
                step.config.queryParams = queryParams;
            }

            console.log('[PropertiesPanel] ✅ Saved query params from global builder to step.config.' + fieldKey + ':', queryParams);
        } else {
            // Normal path: found containers in DOM
            queryParamBuilderContainers.forEach((container, index) => {
                const fieldKey = container.dataset.fieldKey;
                const builder = container._queryParamBuilderInstance;
                console.log(`[PropertiesPanel] 📦 Container #${index} fieldKey="${fieldKey}", has builder:`, !!builder);

                if (builder) {
                    step.config = step.config || {};
                    const queryParams = builder.getParams();
                    console.log(`[PropertiesPanel] 🔑 getParams() returned:`, queryParams);

                    // Store with both the builder key and the backend key for compatibility
                    step.config[fieldKey] = queryParams;  // e.g., queryParamsBuilder

                    // For database enrichment, also store as 'queryParams' for backend compatibility
                    if (fieldKey === 'queryParamsBuilder') {
                        step.config.queryParams = queryParams;
                    }

                    console.log('[PropertiesPanel] ✅ Saved query params to step.config.' + fieldKey + ':', queryParams);
                } else {
                    console.warn(`[PropertiesPanel] ⚠️ Container #${index} fieldKey="${fieldKey}" has NO _queryParamBuilderInstance!`);
                }
            });
        }

        // Collect result mappings from ResultMappingBuilder component (Database Enrichment)
        const resultMappingBuilderContainers = form.querySelectorAll('.result-mapping-builder-container');
        resultMappingBuilderContainers.forEach(container => {
            const builder = container._resultMappingBuilderInstance;
            if (builder) {
                step.config = step.config || {};
                const fieldKey = container.dataset.fieldKey;
                const resultMappings = builder.getMappings();

                // Store with both the builder key and the backend key for compatibility
                step.config[fieldKey] = resultMappings;  // e.g., resultMappingBuilder

                // For database enrichment, also store as 'resultMapping' for backend compatibility
                if (fieldKey === 'resultMappingBuilder') {
                    step.config.resultMapping = resultMappings;
                }

                console.log('[PropertiesPanel] ✅ Saved result mappings to step.config.' + fieldKey + ':', resultMappings);
            }
        });

        // Collect filter from MongoDBFilterBuilder component
        const mongoFilterBuilderContainers = form.querySelectorAll('.mongodb-filter-builder-container');
        mongoFilterBuilderContainers.forEach(container => {
            const builder = container._mongodbFilterBuilderInstance;
            if (builder) {
                step.config = step.config || {};
                const fieldKey = container.dataset.fieldKey;
                const filter = builder.getFilter();

                // Store with both the builder key and the backend key
                step.config[fieldKey] = filter;  // e.g., mongodbFilterBuilder

                // Also store as 'filter' for backend compatibility
                if (fieldKey === 'mongodbFilterBuilder') {
                    step.config.filter = filter;
                }

                console.log('[PropertiesPanel] ✅ Saved MongoDB filter to step.config.' + fieldKey + ':', filter);
            }
        });

        // Collect projection from MongoDBProjectionBuilder component
        const mongoProjectionBuilderContainers = form.querySelectorAll('.mongodb-projection-builder-container');
        mongoProjectionBuilderContainers.forEach(container => {
            const builder = container._mongodbProjectionBuilderInstance;
            if (builder) {
                step.config = step.config || {};
                const fieldKey = container.dataset.fieldKey;
                const projection = builder.getProjection();

                // Store with both the builder key and the backend key
                step.config[fieldKey] = projection;  // e.g., mongodbProjectionBuilder

                // Also store as 'projection' for backend compatibility
                if (fieldKey === 'mongodbProjectionBuilder') {
                    step.config.projection = projection;
                }

                console.log('[PropertiesPanel] ✅ Saved MongoDB projection to step.config.' + fieldKey + ':', projection);
            }
        });

        // Collect Redis query from RedisQueryBuilder component
        const redisQueryBuilderContainers = form.querySelectorAll('.redis-query-builder-container');
        redisQueryBuilderContainers.forEach(container => {
            const builder = container._redisQueryBuilderInstance;
            if (builder) {
                step.config = step.config || {};
                const fieldKey = container.dataset.fieldKey;
                const config = builder.getConfig();

                // Store all Redis fields for backend compatibility
                step.config.redisKey = config.redisKey;           // Key pattern
                step.config.redisCommand = config.redisCommand;   // Command (GET, HGETALL, etc.)
                step.config.redisHashField = config.redisHashField; // Hash field for HGET
                step.config.redisQuery = config.redisQuery;       // Full command string (for display)

                console.log('[PropertiesPanel] ✅ Saved Redis config:', {
                    redisKey: config.redisKey,
                    redisCommand: config.redisCommand,
                    redisHashField: config.redisHashField,
                    redisQuery: config.redisQuery
                });
            }
        });

        // Collect Basic Auth data from auth container
        const basicAuthContainer = form.querySelector('.basic-auth-container');
        if (basicAuthContainer && !basicAuthContainer.closest('.conditional-field.hidden')) {
            const username = basicAuthContainer.querySelector('[name="basicAuth_username"]')?.value || '';
            const password = basicAuthContainer.querySelector('[name="basicAuth_password"]')?.value || '';
            step.config.basicAuth = { username, password };
            // Also store in legacy fields for backward compatibility
            step.config.basicAuthUsername = username;
            step.config.basicAuthPassword = password;
            console.log('[PropertiesPanel] ✅ Saved Basic Auth config');
        }

        // Collect Bearer Token data from auth container
        const bearerTokenContainer = form.querySelector('.bearer-token-container');
        if (bearerTokenContainer && !bearerTokenContainer.closest('.conditional-field.hidden')) {
            const token = bearerTokenContainer.querySelector('[name="bearerToken_token"]')?.value || '';
            step.config.bearerToken = token;
            console.log('[PropertiesPanel] ✅ Saved Bearer Token config');
        }

        // Collect API Key data from auth container
        const apiKeyContainer = form.querySelector('.apikey-container');
        if (apiKeyContainer && !apiKeyContainer.closest('.conditional-field.hidden')) {
            const apiKey = apiKeyContainer.querySelector('[name="apiKeyAuth_apiKey"]')?.value || '';
            const headerName = apiKeyContainer.querySelector('[name="apiKeyAuth_headerName"]')?.value || 'X-API-Key';
            step.config.apiKeyAuth = { apiKey, headerName };
            // Also store in legacy fields for backward compatibility
            step.config.apiKey = apiKey;
            step.config.apiKeyHeader = headerName;
            console.log('[PropertiesPanel] ✅ Saved API Key config');
        }

        // Collect OAuth 2.0 config from OAuth2ConfigBuilder component
        const oauth2ConfigBuilderContainers = form.querySelectorAll('.oauth2-config-builder-container');
        console.log('[PropertiesPanel] 🔍 Found OAuth2 containers:', oauth2ConfigBuilderContainers.length);
        oauth2ConfigBuilderContainers.forEach(container => {
            const builder = container._oauth2ConfigBuilderInstance;
            console.log('[PropertiesPanel] 🔍 OAuth2ConfigBuilder instance:', builder);
            if (builder) {
                step.config = step.config || {};
                const oauth2Config = builder.getConfig();
                console.log('[PropertiesPanel] 🔍 OAuth2 config from builder:', oauth2Config);

                // Map OAuth2ConfigBuilder fields to backend model fields
                // Backend expects: oauth2TokenUrl, oauth2ClientId, oauth2ClientSecret,
                //                  oauth2GrantType, oauth2Scope, oauth2Audience, oauth2Username, oauth2Password
                if (oauth2Config.tokenURL) step.config.oauth2TokenUrl = oauth2Config.tokenURL;
                if (oauth2Config.clientID) step.config.oauth2ClientId = oauth2Config.clientID;
                if (oauth2Config.clientSecret) step.config.oauth2ClientSecret = oauth2Config.clientSecret;
                if (oauth2Config.grantType) step.config.oauth2GrantType = oauth2Config.grantType;
                if (oauth2Config.scope) step.config.oauth2Scope = oauth2Config.scope;
                if (oauth2Config.audience) step.config.oauth2Audience = oauth2Config.audience;
                if (oauth2Config.username) step.config.oauth2Username = oauth2Config.username;
                if (oauth2Config.password) step.config.oauth2Password = oauth2Config.password;

                console.log('[PropertiesPanel] ✅ Saved OAuth 2.0 config to step.config (flattened):', {
                    oauth2TokenUrl: step.config.oauth2TokenUrl,
                    oauth2ClientId: step.config.oauth2ClientId,
                    oauth2GrantType: step.config.oauth2GrantType,
                    oauth2Scope: step.config.oauth2Scope
                });
            } else {
                console.warn('[PropertiesPanel] ⚠️ OAuth2ConfigBuilder instance not found on container');
            }
        });

        // Collect If-Then-Else configuration from IfThenElseBuilder component
        // Use VisualStep utility for OOP-compliant type detection
        if (this.ifThenElseBuilder && VisualStep.isIfThenElse(step)) {
            step.config = step.config || {};
            const conditions = this.ifThenElseBuilder.getConfig();
            step.config = conditions; // Replace entire config with conditions object
            console.log('[PropertiesPanel] ✅ Saved If-Then-Else conditions to step.config:', conditions);
        }

        // Collect Connector configuration from ConnectorConfigBuilder component
        if (this.connectorConfigBuilder && VisualStep.isConnectorStep(step)) {
            const connectorConfig = this.connectorConfigBuilder.getConfig();
            step.config = connectorConfig;
            console.log('[PropertiesPanel] ✅ Saved Connector config to step.config:', connectorConfig);
        }

        // Collect FHIR Validation configuration
        if (VisualStep.isFHIRValidation(step)) {
            step.config = step.config || {};

            // Validation level (lowercase values from select)
            const levelSelect = form.querySelector('#fhirValidationLevel');
            if (levelSelect) {
                step.config.validation_level = levelSelect.value;
            }

            // Validate references toggle
            const refsCheckbox = form.querySelector('#fhirValidateReferences');
            if (refsCheckbox) {
                step.config.validate_references = refsCheckbox.checked;
            }

            // FHIR version selector
            const versionSelect = form.querySelector('#fhirVersionSelect');
            if (versionSelect) {
                step.config.fhir_version = versionSelect.value;
            }

            // Profile selector
            const profileSelect = form.querySelector('#fhirProfileSelect');
            if (profileSelect) {
                step.config.profile = profileSelect.value;
            }

            // Required resources: grid checkboxes + custom chips
            const requiredResources = [];
            form.querySelectorAll('input[name="fhir_required_resource"]:checked').forEach(cb => {
                requiredResources.push(cb.value);
            });
            form.querySelectorAll('.fhir-custom-resource-chip').forEach(chip => {
                const resource = chip.dataset.resource;
                if (resource && !requiredResources.includes(resource)) {
                    requiredResources.push(resource);
                }
            });
            step.config.required_resources = requiredResources;

            // Fail on error toggle
            const failOnErrorCheckbox = form.querySelector('#fhirFailOnError');
            if (failOnErrorCheckbox) {
                step.config.fail_on_error = failOnErrorCheckbox.checked;
            }

            console.log('[PropertiesPanel] ✅ Saved FHIR Validation config:', step.config);
        }

        // File Parser / Remove Duplicates / Data Masking / Normalizer / API & DB Enrichment / Payload Builder / CDA steps — delegated to active step builder
        if (this.activeStepBuilder && (
            VisualStep.isFileParser(step) || VisualStep.isRemoveDuplicates(step) ||
            VisualStep.isDataMasking(step) || VisualStep.isNormalizer(step) ||
            VisualStep.isAPIEnrichment(step) || VisualStep.isDatabaseEnrichment(step) ||
            VisualStep.isDeidentify(step) || VisualStep.isPayloadBuilder(step) ||
            VisualStep.isCdaStep(step)
        )) {
            this.activeStepBuilder.collectConfig(step);
        }

        // Collect dynamic configuration fields (enrichment checkboxes, text inputs, etc.)
        step.config = step.config || {};

        // Collect all config_* fields
        const configInputs = form.querySelectorAll('[name^="config_"]');
        configInputs.forEach(input => {
            const fieldName = input.name.replace('config_', '');

            // Skip fields that are hidden (conditionally invisible)
            const fieldContainer = input.closest('.form-group');
            if (fieldContainer && fieldContainer.classList.contains('hidden')) {
                console.log(`[PropertiesPanel] Skipping hidden field: ${fieldName}`);
                return; // Skip this field
            }

            if (input.type === 'checkbox') {
                step.config[fieldName] = input.checked;
            } else if (input.type === 'number') {
                step.config[fieldName] = parseInt(input.value) || 0;
            } else if (input.tagName === 'TEXTAREA') {
                // Try to parse JSON from textareas, otherwise use raw value
                const trimmedValue = input.value.trim();

                // Handle empty textareas - send null instead of empty string to avoid backend unmarshaling errors
                if (trimmedValue === '') {
                    step.config[fieldName] = null;
                } else {
                    try {
                        if (trimmedValue.startsWith('{') || trimmedValue.startsWith('[')) {
                            step.config[fieldName] = JSON.parse(trimmedValue);
                        } else {
                            step.config[fieldName] = trimmedValue;
                        }
                    } catch (e) {
                        step.config[fieldName] = trimmedValue;
                    }
                }
            } else {
                step.config[fieldName] = input.value;
            }
        });

        console.log('[PropertiesPanel] Collected config fields:', step.config);

        // CRITICAL FIX: For database enrichment steps, if individual connection fields are provided,
        // force connectionString to be empty so executor builds it from individual fields
        if (step.config.databaseType &&
            (step.config.dbHost || step.config.dbName || step.config.dbUser)) {
            console.log('[PropertiesPanel] 🔧 Database enrichment with individual fields detected');
            console.log('[PropertiesPanel] 🔧 Setting connectionString to empty to force field-based connection');
            step.config.connectionString = '';
        }

        // Configuration textarea (legacy) - ONLY for steps that use JSON config editor
        const configText = form.querySelector('#stepConfig')?.value;
        if (configText) {
            try {
                const parsedConfig = JSON.parse(configText);
                // CRITICAL: Preserve mappings array if it exists (for field mapping steps)
                if (step.config && step.config.mappings && Array.isArray(step.config.mappings)) {
                    parsedConfig.mappings = step.config.mappings;
                    console.log('[PropertiesPanel] ✅ Preserved mappings array:', step.config.mappings.length, 'items');
                }
                // CRITICAL: Preserve assemblyRules and assembleObservations — these are written
                // by event listeners (radio buttons, checkboxes) directly to step.config and would
                // be wiped out if the raw JSON textarea replaces the whole config object.
                if (step.config && step.config.assemblyRules !== undefined) {
                    parsedConfig.assemblyRules = step.config.assemblyRules;
                }
                if (step.config && step.config.assembleObservations !== undefined) {
                    parsedConfig.assembleObservations = step.config.assembleObservations;
                }
                step.config = parsedConfig;
            } catch (error) {
                throw new Error('Invalid JSON in configuration');
            }
        }

        // FINAL CHECK: Log the final config being saved
        console.log('[PropertiesPanel] 📦 Final step.config being saved:', {
            hasConfig: !!step.config,
            hasMappings: !!(step.config && step.config.mappings),
            mappingsCount: (step.config && step.config.mappings) ? step.config.mappings.length : 0,
            configKeys: step.config ? Object.keys(step.config) : []
        });

        // Error Handling (universal - applies to all steps)
        const errorHandlingSection = form.querySelector('.error-handling-section');
        if (errorHandlingSection) {
            const ehEnabled = errorHandlingSection.querySelector('#ehEnabled');
            if (ehEnabled) {
                step.config = step.config || {};
                if (ehEnabled.checked) {
                    const onError = errorHandlingSection.querySelector('#ehOnError')?.value || 'suppress';
                    const defaultField = errorHandlingSection.querySelector('#ehDefaultField')?.value?.trim() || '';
                    const defaultValue = errorHandlingSection.querySelector('#ehDefaultValue')?.value?.trim() || '';

                    step.config.errorHandling = {
                        enabled: true,
                        onError: onError
                    };
                    // Only store default value if both field and value are provided
                    if (defaultField && defaultValue) {
                        step.config.errorHandling.defaultField = defaultField;
                        step.config.errorHandling.defaultValue = defaultValue;
                    }
                    console.log('[PropertiesPanel] Saved Error Handling config:', step.config.errorHandling);
                } else {
                    delete step.config.errorHandling;
                }
            }

            // Retry config (per-step retry)
            const retryEnabled = errorHandlingSection.querySelector('#retryEnabled');
            if (retryEnabled) {
                if (retryEnabled.checked) {
                    step.config.retry = {
                        enabled: true,
                        maxRetries: parseInt(errorHandlingSection.querySelector('#retryMaxRetries')?.value) || 3,
                        delayMs: parseInt(errorHandlingSection.querySelector('#retryDelayMs')?.value) || 1000,
                        backoffMultiplier: parseFloat(errorHandlingSection.querySelector('#retryBackoffMultiplier')?.value) || 2
                    };
                    console.log('[PropertiesPanel] Saved Retry config:', step.config.retry);
                } else {
                    delete step.config.retry;
                }
            }
        }

        // Script (if present)
        const scriptType = form.querySelector('#scriptType')?.value;
        const scriptContent = form.querySelector('#scriptContent')?.value;
        if (scriptType) step.scriptType = scriptType;
        if (scriptContent) step.scriptContent = scriptContent;

        return step;
    }

    /**
     * Validate JSON input
     */
    validateJSON(textarea) {
        try {
            JSON.parse(textarea.value);
            textarea.style.borderColor = '';
        } catch (error) {
            textarea.style.borderColor = '#ef4444';
            this.builder.dragDropManager.showNotification('Invalid JSON', 'error');
        }
    }

    /**
     * Validate script for syntax errors and dependency issues
     */
    async validateScript(step) {
        const scriptContent = document.getElementById('scriptContent');
        const validateBtn = document.getElementById('validateScriptBtn');
        const resultDiv = document.getElementById('scriptValidationResult');

        if (!scriptContent || !scriptContent.value.trim()) {
            resultDiv.innerHTML = '<span style="color: #ef4444;">⚠️ Script is empty</span>';
            return;
        }

        // Show loading state
        validateBtn.disabled = true;
        validateBtn.innerHTML = '⏳ Validating...';
        resultDiv.innerHTML = '<span style="color: #64748b;">Checking script...</span>';

        try {
            const response = await fetch('/api/fhir/pipeline/validate-script', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    script: scriptContent.value,
                    pipelineId: this.builder.currentPipelineId
                })
            });

            const result = await response.json();

            if (result.success) {
                resultDiv.innerHTML = '<span style="color: #10b981;">✅ Script is valid!</span>';
                scriptContent.style.borderColor = '#10b981';
                this.builder.dragDropManager.showNotification('Script validation passed', 'success');
            } else {
                const errorMsg = result.error || 'Unknown validation error';
                resultDiv.innerHTML = `<span style="color: #ef4444; font-weight: 500;">❌ ${errorMsg}</span>`;
                scriptContent.style.borderColor = '#ef4444';
                // Show as a modal prompt so the error is always readable
                this.builder.dragDropManager.showErrorDialog(errorMsg, 'Script Validation Failed');

                // Log detailed errors to console if available
                if (result.details) {
                    console.error('Script validation details:', result.details);
                }
            }
        } catch (error) {
            console.error('Script validation error:', error);
            resultDiv.innerHTML = `<span style="color: #ef4444; font-weight: 500;">❌ ${error.message}</span>`;
            this.builder.dragDropManager.showErrorDialog(error.message, 'Script Validation Error');
        } finally {
            validateBtn.disabled = false;
            validateBtn.innerHTML = '🔍 Validate Script';
        }
    }

    /**
     * Silently refresh pipelineLastTestOutput after a step is saved/opened.
     *
     * Sample message resolution order (first non-empty wins):
     *  1. Live test modal input (user just typed something)
     *  2. localStorage — persisted from the last manual test run
     *  3. Last real message received by this interface (fetched from the API)
     *     — this means: as long as one real message has ever come in, the path
     *       picker self-populates with no user action required at all.
     */
    async _silentRefreshTestOutput() {
        if (!this.builder?.pipeline) return;

        // Source 1: live modal input
        const messageInput = document.getElementById('testMessageInput');
        let sampleMessage = messageInput?.value?.trim() || '';

        // Source 2: persisted from previous test run
        if (!sampleMessage) {
            try { sampleMessage = localStorage.getItem('pipeline_last_sample_message') || ''; } catch (_) {}
        }

        // Source 3: last real message received by this interface
        if (!sampleMessage) {
            try {
                const interfaceId = this.builder?.interfaceId
                    || new URLSearchParams(window.location.search).get('interfaceId');
                if (interfaceId) {
                    const resp = await fetch(`/api/messages/interface/${interfaceId}?limit=1&order=received_at+DESC`);
                    if (resp.ok) {
                        const data = await resp.json();
                        const lastMsg = Array.isArray(data.messages) ? data.messages[0] : null;
                        if (lastMsg?.raw_message) {
                            sampleMessage = lastMsg.raw_message;
                            // Persist so future refreshes skip this fetch
                            try { localStorage.setItem('pipeline_last_sample_message', sampleMessage); } catch (_) {}
                        }
                    }
                }
            } catch (_) {}
        }

        if (!sampleMessage) return;

        // Build a partial pipeline copy with remove_duplicates stripped so the test
        // succeeds even when those steps have an unconfigured or invalid sourceField.
        const makePartialPipeline = () => {
            try {
                const data = JSON.parse(JSON.stringify(this.builder.pipeline.toJSON()));
                for (const group of (data.execution_groups || [])) {
                    group.steps = (group.steps || []).filter(s => {
                        const t = s.step_type || s.stepType || s.type || '';
                        return t !== 'remove_duplicates';
                    });
                }
                return { toJSON: () => data };
            } catch { return null; }
        };

        try {
            const result = await window.pipelineAPI.testPipeline(this.builder.pipeline, sampleMessage);
            if (result?.steps) {
                window.pipelineLastTestOutput = result;
                console.log('[PropertiesPanel] Silently refreshed test output after step save:', Object.keys(result.steps));
                try { localStorage.setItem('pipeline_last_test_output', JSON.stringify(result)); } catch (_) {}
            }
        } catch (err) {
            // Full pipeline failed — try partial run without remove_duplicates
            console.debug('[PropertiesPanel] Full pipeline test failed, trying partial run:', err.message);
            try {
                const partial = makePartialPipeline();
                if (!partial) return;
                const result = await window.pipelineAPI.testPipeline(partial, sampleMessage);
                if (result?.steps) {
                    window.pipelineLastTestOutput = result;
                    console.log('[PropertiesPanel] Silently refreshed test output (partial run):', Object.keys(result.steps));
                    try { localStorage.setItem('pipeline_last_test_output', JSON.stringify(result)); } catch (_) {}
                }
            } catch (err2) {
                console.debug('[PropertiesPanel] Partial silent refresh also failed:', err2.message);
            }
        }
    }

    /**
     * Hide properties panel
     */
    hideProperties() {
        // Prevent infinite loop with StepNodeManager.deselectNode()
        if (this.isHiding) return;
        this.isHiding = true;

        try {
            this.currentStep = null;
            this.container.innerHTML = `
                <div class="no-selection-message">
                    <i class="fas fa-mouse-pointer"></i>
                    <p>Select a step to configure its properties</p>
                </div>
            `;
            this.builder.stepNodeManager.deselectNode();
        } finally {
            this.isHiding = false;
        }
    }

    /**
     * Show pipeline properties
     */
    showPipelineProperties(pipeline) {
        this.container.innerHTML = `
            <div class="properties-form">
                <h3><i class="fas fa-project-diagram"></i> Pipeline Settings</h3>

                <div class="form-section">
                    <div class="form-group">
                        <label>Pipeline Name</label>
                        <input type="text" id="pipelineName" value="${pipeline.name}">
                    </div>

                    <div class="form-group">
                        <label>Description</label>
                        <textarea id="pipelineDescription" rows="3">${pipeline.description || ''}</textarea>
                    </div>

                    <div class="form-group">
                        <label>Status</label>
                        <select id="pipelineStatus">
                            <option value="draft" ${pipeline.status === 'draft' ? 'selected' : ''}>Draft</option>
                            <option value="active" ${pipeline.status === 'active' ? 'selected' : ''}>Active</option>
                            <option value="paused" ${pipeline.status === 'paused' ? 'selected' : ''}>Paused</option>
                        </select>
                    </div>

                    <button class="btn btn-primary" id="savePipelineSettings">
                        <i class="fas fa-save"></i> Save Settings
                    </button>
                </div>
            </div>
        `;

        // Attach save handler
        const saveBtn = this.container.querySelector('#savePipelineSettings');
        if (saveBtn) {
            saveBtn.addEventListener('click', () => {
                pipeline.name = this.container.querySelector('#pipelineName').value;
                pipeline.description = this.container.querySelector('#pipelineDescription').value;
                pipeline.status = this.container.querySelector('#pipelineStatus').value;
                this.builder.markAsUnsaved();
                this.builder.dragDropManager.showNotification('Pipeline settings updated', 'success');
            });
        }
    }

    /**
     * Create dynamic form fields based on step type
     */
    createDynamicFormFields(step) {
        const stepType = step.stepType || step.type;

        // DEBUG: Log step details to understand why builder not appearing
        // Use VisualStep utilities for OOP-compliant type detection
        console.log('🔍 createDynamicFormFields called with:', {
            stepType,
            templateId: step.templateId,
            stepName: step.stepName,
            checkingIfThenElse: VisualStep.isIfThenElse(step)
        });

        // Special handling for If-Then-Else conditional logic
        // Use VisualStep.isIfThenElse() for centralized type detection
        if (VisualStep.isIfThenElse(step)) {
            console.log('🎨 Using If-Then-Else Builder for conditional logic');
            return this.createIfThenElseUI(step);
        }

        // Special handling for Switch/Case conditional logic
        if (VisualStep.isSwitchCase(step)) {
            console.log('🎨 Using Switch/Case Builder for conditional logic');
            return this.createSwitchCaseUI(step);
        }

        // Special handling for Loop container step
        if (VisualStep.isLoopStep(step)) {
            console.log('🎨 Using ForEachLoop Builder for loop container');
            return this.createLoopContainerUI(step);
        }

        // Special handling for HL7→FHIR mapping steps ONLY (not generic transformation)
        if (VisualStep.isHL7FHIRTransform(step)) {
            console.log('🎨 Using enhanced HL7→FHIR mapping UI for step type:', stepType);
            return this.createMappingConfigSection(step);
        }

        // Special handling for generic field transformation/mapping (includes both field mapping and metadata)
        if (VisualStep.isFieldMapping(step) || stepType === 'pre.enrichment.metadata') {
            console.log('🎨 Using unified field mapping UI for:', stepType);
            return this.createGenericFieldMappingSection(step);
        }

        // Special handling for Connector steps (Inbound/Outbound)
        if (VisualStep.isConnectorStep(step)) {
            const direction = VisualStep.isInboundConnector(step) ? 'inbound' : 'outbound';
            console.log(`🔌 Using ConnectorConfigBuilder for ${direction} connector`);
            return this.createConnectorConfigUI(step, direction);
        }

        // Special handling for FHIR Validation step
        if (VisualStep.isFHIRValidation(step)) {
            console.log('🛡️ Using enhanced FHIR Validation UI for step type:', stepType);
            return this.createFHIRValidationUI(step);
        }

        // Special handling for File Parser step (fixed-width column builder)
        if (VisualStep.isFileParser(step)) {
            console.log('📄 Using File Parser UI with column definition builder');
            if (this.activeStepBuilder) { this.activeStepBuilder.destroy(); }
            this.activeStepBuilder = StepBuilderRegistry.create('file_parser', this);
            return this.activeStepBuilder.render(step);
        }

        // Special handling for Remove Duplicates step (chip-based no-code UI)
        if (VisualStep.isRemoveDuplicates(step)) {
            console.log('🔧 Using Remove Duplicates no-code UI');
            if (this.activeStepBuilder) { this.activeStepBuilder.destroy(); }
            this.activeStepBuilder = StepBuilderRegistry.create('remove_duplicates', this);
            return this.activeStepBuilder.render(step);
        }

        // Special handling for Data Masking step (rule-based no-code UI)
        if (VisualStep.isDataMasking(step)) {
            console.log('🔒 Using Data Masking no-code UI');
            if (this.activeStepBuilder) { this.activeStepBuilder.destroy(); }
            this.activeStepBuilder = StepBuilderRegistry.create('data_masking', this);
            return this.activeStepBuilder.render(step);
        }

        if (VisualStep.isNormalizer(step)) {
            console.log('📐 Using Normalizer / Pivot / Transpose no-code UI');
            if (this.activeStepBuilder) { this.activeStepBuilder.destroy(); }
            this.activeStepBuilder = StepBuilderRegistry.create('normalizer', this);
            return this.activeStepBuilder.render(step);
        }

        if (VisualStep.isAPIEnrichment(step)) {
            console.log('🌐 Using API Enrichment no-code UI');
            if (this.activeStepBuilder) { this.activeStepBuilder.destroy(); }
            this.activeStepBuilder = StepBuilderRegistry.create('enrichment.api', this);
            return this.activeStepBuilder.render(step);
        }

        if (VisualStep.isDatabaseEnrichment(step)) {
            console.log('🗄️ Using Database Enrichment no-code UI');
            if (this.activeStepBuilder) { this.activeStepBuilder.destroy(); }
            this.activeStepBuilder = StepBuilderRegistry.create('database_enrichment', this);
            return this.activeStepBuilder.render(step);
        }

        if (VisualStep.isDeidentify(step)) {
            console.log('🛡️ Using HIPAA De-identify no-code UI');
            if (this.activeStepBuilder) { this.activeStepBuilder.destroy(); }
            this.activeStepBuilder = StepBuilderRegistry.create('deidentify', this);
            return this.activeStepBuilder.render(step);
        }

        if (VisualStep.isPayloadBuilder(step)) {
            console.log('📤 Using Payload Builder no-code UI');
            if (this.activeStepBuilder) { this.activeStepBuilder.destroy(); }
            this.activeStepBuilder = StepBuilderRegistry.create('payload.builder', this);
            return this.activeStepBuilder.render(step);
        }

        if (VisualStep.isCdaParseStep(step)) {
            console.log('🏥 Using CDA Parse step builder');
            if (this.activeStepBuilder) { this.activeStepBuilder.destroy(); }
            this.activeStepBuilder = StepBuilderRegistry.create('cda.parse', this);
            return this.activeStepBuilder.render(step);
        }

        if (VisualStep.isCdaToFhirStep(step)) {
            console.log('🏥 Using CDA→FHIR 4-tab step builder');
            if (this.activeStepBuilder) { this.activeStepBuilder.destroy(); }
            this.activeStepBuilder = StepBuilderRegistry.create('cda.to_fhir', this);
            return this.activeStepBuilder.render(step);
        }

        if (VisualStep.isCdaStep(step)) {
            // fhir.to_cda and cda.normalize also use StepBuilderRegistry
            const type = step.stepType || step.step_type || '';
            if (this.activeStepBuilder) { this.activeStepBuilder.destroy(); }
            this.activeStepBuilder = StepBuilderRegistry.create(type, this);
            if (this.activeStepBuilder) {
                return this.activeStepBuilder.render(step);
            }
        }

        // Destroy any registered builder when falling through to generic UI
        if (this.activeStepBuilder) { this.activeStepBuilder.destroy(); this.activeStepBuilder = null; }

        // Get step-specific configuration
        const stepConfig = this.getStepConfiguration(stepType);

        if (!stepConfig || !stepConfig.fields) {
            return ''; // No dynamic fields for this step type
        }

        let html = `
            <div class="form-section">
                <h4>Step-Specific Configuration</h4>
        `;

        stepConfig.fields.forEach(field => {
            const rawValue = step.config?.[field.key] || field.default || '';

            // DEBUG: Log script field value for Script Enrichment steps
            if (field.key === 'script' && VisualStep.isScriptEnrichment(step)) {
                console.log('🐛 DEBUG Script field:');
                console.log('   step.config:', step.config);
                console.log('   step.config.script:', step.config?.script);
                console.log('   field.default:', field.default);
                console.log('   rawValue (first 100 chars):', typeof rawValue === 'string' ? rawValue.substring(0, 100) : rawValue);
            }

            // For textareas that expect JSON, stringify objects/arrays
            const value = (field.type === 'textarea' && (typeof rawValue === 'object' && rawValue !== null))
                ? JSON.stringify(rawValue, null, 2)
                : rawValue;

            // Handle conditional visibility (can have both visibleWhen AND showIf)
            let visibilityClass = '';
            let visibilityDataAttr = '';
            let shouldHide = false;

            // Check visibleWhen condition (single value match)
            if (field.visibleWhen) {
                visibilityClass = 'conditional-field';
                visibilityDataAttr = `data-visible-when-field="${field.visibleWhen.field}" data-visible-when-value="${field.visibleWhen.value}"`;

                const controlFieldValue = step.config?.[field.visibleWhen.field] ||
                                         stepConfig.fields.find(f => f.key === field.visibleWhen.field)?.default || '';
                if (controlFieldValue !== field.visibleWhen.value) {
                    shouldHide = true;
                }
            }

            // Check showIf condition (multiple value match - AND with visibleWhen if both present)
            if (field.showIf) {
                visibilityClass = 'conditional-field';
                const showIfAttr = `data-show-if-field="${field.showIf.field}" data-show-if-values='${JSON.stringify(field.showIf.values)}'`;

                // Combine with existing visibility data if present
                visibilityDataAttr = visibilityDataAttr ? `${visibilityDataAttr} ${showIfAttr}` : showIfAttr;

                const controlFieldValue = step.config?.[field.showIf.field] ||
                                         stepConfig.fields.find(f => f.key === field.showIf.field)?.default || '';
                if (!field.showIf.values.includes(controlFieldValue)) {
                    shouldHide = true;
                }
            }

            // Apply hidden class if any condition fails
            if (shouldHide) {
                visibilityClass += ' hidden';
            }

            html += `<div class="form-group ${visibilityClass}" ${visibilityDataAttr}>`;
            html += `<label>${field.label}${field.required ? ' *' : ''}</label>`;

            switch (field.type) {
                case 'text':
                case 'password':
                    html += `<input type="${field.type}" name="config_${field.key}" value="${value}" ${field.required ? 'required' : ''} placeholder="${field.placeholder || ''}">`;
                    break;

                case 'number':
                    html += `<input type="number" name="config_${field.key}" value="${value}" ${field.required ? 'required' : ''} min="${field.min || 0}" max="${field.max || ''}" step="${field.step || 1}">`;
                    break;

                case 'textarea':
                    html += `<textarea name="config_${field.key}" rows="${field.rows || 3}" ${field.required ? 'required' : ''} placeholder="${field.placeholder || ''}">${value}</textarea>`;
                    break;

                case 'select':
                    html += `<select name="config_${field.key}" ${field.required ? 'required' : ''}>`;
                    field.options.forEach(opt => {
                        const selected = value === opt.value ? 'selected' : '';
                        html += `<option value="${opt.value}" ${selected}>${opt.label}</option>`;
                    });
                    html += `</select>`;
                    break;

                case 'checkbox': {
                    // Use explicit config value when present; only fall back to field.default for undefined.
                    // This prevents `false` being overridden by `default: true` via the `||` short-circuit.
                    const boolVal = step.config?.[field.key] !== undefined ? step.config[field.key] : field.default;
                    const checked = boolVal === true || boolVal === 'true' ? 'checked' : '';
                    html += `<label class="checkbox-label"><input type="checkbox" name="config_${field.key}" ${checked}> ${field.checkboxLabel || field.label}</label>`;
                    break;
                }

                case 'multiselect':
                    html += `<div class="multiselect-group">`;
                    field.options.forEach(opt => {
                        const valueArray = Array.isArray(value) ? value : [];
                        const checked = valueArray.includes(opt.value) ? 'checked' : '';
                        html += `<label class="checkbox-label"><input type="checkbox" name="config_${field.key}[]" value="${opt.value}" ${checked}> ${opt.label}</label>`;
                    });
                    html += `</div>`;
                    break;

                case 'validation-builder':
                    // Use modular ValidationRuleBuilder component
                    // Parse existing rules
                    let rules = [];
                    try {
                        if (typeof value === 'string') {
                            rules = JSON.parse(value);
                        } else if (Array.isArray(value)) {
                            rules = value;
                        }
                    } catch (e) {
                        console.warn('Failed to parse validation rules:', e);
                        rules = [];
                    }

                    // Create container for ValidationRuleBuilder component
                    html += `<div class="validation-builder-container" data-field-key="${field.key}" data-initial-rules='${JSON.stringify(rules)}'></div>`;
                    break;

                case 'metadata-builder':
                    // Key-value pair builder for custom metadata
                    let metadata = {};
                    try {
                        if (typeof value === 'string') {
                            metadata = JSON.parse(value);
                        } else if (typeof value === 'object' && value !== null) {
                            metadata = value;
                        }
                    } catch (e) {
                        console.warn('Failed to parse metadata:', e);
                        metadata = {};
                    }

                    // Create container for MetadataBuilder component
                    html += `<div class="metadata-builder-container" data-field-key="${field.key}" data-initial-metadata='${JSON.stringify(metadata)}'></div>`;
                    break;

                case 'header-builder':
                    // HTTP Header builder for API enrichment
                    let headers = {};
                    try {
                        if (typeof value === 'string') {
                            headers = JSON.parse(value);
                        } else if (typeof value === 'object' && value !== null) {
                            headers = value;
                        }
                    } catch (e) {
                        console.warn('Failed to parse headers:', e);
                        headers = {};
                    }

                    // Create container for HeaderBuilder component
                    html += `<div class="header-builder-container" data-field-key="${field.key}" data-initial-headers='${JSON.stringify(headers)}'></div>`;
                    break;

                case 'query-param-builder':
                    // Query parameter builder for API enrichment and database enrichment
                    let queryParams = {};
                    try {
                        if (typeof value === 'string' && value.trim() !== '') {
                            queryParams = JSON.parse(value);
                        } else if (typeof value === 'object' && value !== null) {
                            queryParams = value;
                        }
                    } catch (e) {
                        console.warn('Failed to parse query params:', e);
                        queryParams = {};
                    }

                    console.log(`[PropertiesPanel] 🏗️ Rendering query-param-builder for field "${field.key}", value from step.config:`, value, 'parsed queryParams:', queryParams);

                    // Create container for QueryParamBuilder component
                    html += `<div class="query-param-builder-container" data-field-key="${field.key}" data-initial-params='${JSON.stringify(queryParams)}'></div>`;
                    break;

                case 'result-mapping-builder':
                    // Result mapping builder for database enrichment - NO-CODE visual mapper
                    let resultMappings = {};
                    try {
                        if (typeof value === 'string') {
                            resultMappings = JSON.parse(value);
                        } else if (typeof value === 'object' && value !== null) {
                            resultMappings = value;
                        }
                    } catch (e) {
                        console.warn('Failed to parse result mappings:', e);
                        resultMappings = {};
                    }

                    // Create container for ResultMappingBuilder component
                    html += `<div class="result-mapping-builder-container" data-field-key="${field.key}" data-initial-mappings='${JSON.stringify(resultMappings)}'></div>`;
                    break;

                case 'api-endpoint-tester':
                    // API Endpoint Tester - NO-CODE: Test API and visually pick response fields
                    // This enables first-time users to see actual API response before configuration
                    html += `<div class="api-endpoint-tester-container" id="api-endpoint-tester-container"></div>`;
                    break;

                case 'api-response-mapping-builder':
                    // API Response Mapping Builder - Shows configured output variable mappings
                    let responseMapping = { extractors: [] };
                    try {
                        if (typeof value === 'string') {
                            responseMapping = JSON.parse(value);
                        } else if (typeof value === 'object' && value !== null) {
                            responseMapping = value;
                        }
                    } catch (e) {
                        responseMapping = { extractors: [] };
                    }

                    // Create container for API Response Mapping Builder component
                    html += `<div class="api-response-mapping-builder-container" data-field-key="${field.key}" data-initial-mappings='${JSON.stringify(responseMapping)}'></div>`;
                    break;

                case 'database-query-tester':
                    // Database Query Tester - NO-CODE: Test SQL queries before saving pipeline
                    // Shows actual database results and allows click-to-add mapping
                    html += `<div class="database-query-tester-container" data-field-key="${field.key}"></div>`;
                    break;

                case 'mongodb-filter-builder':
                    // MongoDB Filter Builder - NO-CODE visual query builder
                    let filterValue = {};
                    try {
                        if (typeof value === 'string') {
                            filterValue = JSON.parse(value);
                        } else if (typeof value === 'object' && value !== null) {
                            filterValue = value;
                        }
                    } catch (e) {
                        console.warn('Failed to parse MongoDB filter:', e);
                        filterValue = {};
                    }

                    html += `<div class="mongodb-filter-builder-container" data-field-key="${field.key}" data-initial-filter='${JSON.stringify(filterValue)}'></div>`;
                    break;

                case 'mongodb-projection-builder':
                    // MongoDB Projection Builder - NO-CODE field selector
                    let projectionValue = {};
                    try {
                        if (typeof value === 'string') {
                            projectionValue = JSON.parse(value);
                        } else if (typeof value === 'object' && value !== null) {
                            projectionValue = value;
                        }
                    } catch (e) {
                        console.warn('Failed to parse MongoDB projection:', e);
                        projectionValue = {};
                    }

                    html += `<div class="mongodb-projection-builder-container" data-field-key="${field.key}" data-initial-projection='${JSON.stringify(projectionValue)}'></div>`;
                    break;

                case 'redis-query-builder':
                    // Redis Query Builder - NO-CODE visual query builder
                    // Get Redis config from step.config (backend format)
                    const redisCommand = step.config?.redisCommand || '';
                    const redisKey = step.config?.redisKey || '';
                    const redisHashField = step.config?.redisHashField || '';
                    const redisQuery = step.config?.redisQuery || '';

                    // Escape function for safe attributes
                    const escapeAttr = (str) => String(str || '')
                        .replace(/&/g, '&amp;')
                        .replace(/'/g, '&#39;')
                        .replace(/"/g, '&quot;')
                        .replace(/</g, '&lt;')
                        .replace(/>/g, '&gt;');

                    html += `<div class="redis-query-builder-container"
                                  data-field-key="${field.key}"
                                  data-redis-query='${escapeAttr(redisQuery)}'
                                  data-redis-command='${escapeAttr(redisCommand)}'
                                  data-redis-key='${escapeAttr(redisKey)}'
                                  data-redis-hash-field='${escapeAttr(redisHashField)}'></div>`;
                    break;

                case 'basic-auth-container':
                    // Basic authentication container
                    let basicAuthData = {};
                    try {
                        if (typeof value === 'string') {
                            basicAuthData = JSON.parse(value);
                        } else if (typeof value === 'object' && value !== null) {
                            basicAuthData = value;
                        }
                    } catch (e) {
                        basicAuthData = {};
                    }

                    // Create styled container for basic auth
                    html += `<div class="auth-container basic-auth-container">
                        <div class="auth-container-header">
                            <h4><i class="fas fa-user-lock"></i> Basic Authentication</h4>
                        </div>
                        <div class="auth-container-body">
                            <div class="form-group">
                                <label>Username <span class="text-danger">*</span></label>
                                <input type="text" class="form-control" name="basicAuth_username"
                                       value="${basicAuthData.username || ''}"
                                       placeholder="username" required>
                                <small class="form-text text-muted">Username for basic authentication</small>
                            </div>
                            <div class="form-group">
                                <label>Password <span class="text-danger">*</span></label>
                                <input type="password" class="form-control" name="basicAuth_password"
                                       value="${basicAuthData.password || ''}"
                                       placeholder="password" required>
                                <small class="form-text text-muted">Password for basic authentication</small>
                            </div>
                        </div>
                    </div>`;
                    break;

                case 'bearer-token-container':
                    // Bearer token container
                    let bearerTokenData = typeof value === 'string' ? value : (value?.token || '');

                    // Create styled container for bearer token
                    html += `<div class="auth-container bearer-token-container">
                        <div class="auth-container-header">
                            <h4><i class="fas fa-key"></i> Bearer Token Authentication</h4>
                        </div>
                        <div class="auth-container-body">
                            <div class="form-group">
                                <label>Bearer Token <span class="text-danger">*</span></label>
                                <textarea class="form-control" name="bearerToken_token" rows="4"
                                          placeholder="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." required>${bearerTokenData}</textarea>
                                <small class="form-text text-muted">JWT or access token for Bearer authentication. Typically starts with "eyJ"</small>
                            </div>
                            <div class="form-group">
                                <label class="checkbox-label">
                                    <input type="checkbox" name="bearerToken_testConnection">
                                    Test token validity on save
                                </label>
                            </div>
                        </div>
                    </div>`;
                    break;

                case 'apikey-container':
                    // API key container
                    let apiKeyData = {};
                    try {
                        if (typeof value === 'string') {
                            apiKeyData = JSON.parse(value);
                        } else if (typeof value === 'object' && value !== null) {
                            apiKeyData = value;
                        }
                    } catch (e) {
                        apiKeyData = {};
                    }

                    // Create styled container for API key
                    html += `<div class="auth-container apikey-container">
                        <div class="auth-container-header">
                            <h4><i class="fas fa-fingerprint"></i> API Key Authentication</h4>
                        </div>
                        <div class="auth-container-body">
                            <div class="form-group">
                                <label>API Key <span class="text-danger">*</span></label>
                                <input type="text" class="form-control" name="apiKeyAuth_apiKey"
                                       value="${apiKeyData.apiKey || ''}"
                                       placeholder="your-api-key-here" required>
                                <small class="form-text text-muted">API key provided by the service</small>
                            </div>
                            <div class="form-group">
                                <label>Header Name</label>
                                <input type="text" class="form-control" name="apiKeyAuth_headerName"
                                       value="${apiKeyData.headerName || 'X-API-Key'}"
                                       placeholder="X-API-Key">
                                <small class="form-text text-muted">HTTP header name for the API key (default: X-API-Key)</small>
                            </div>
                            <div class="common-headers-hint">
                                <strong>Common header names:</strong> X-API-Key, X-API-Token, Authorization, api-key
                            </div>
                        </div>
                    </div>`;
                    break;

                case 'oauth2-builder':
                    // OAuth 2.0 configuration builder
                    let oauth2Config = {};
                    try {
                        if (typeof value === 'string') {
                            oauth2Config = JSON.parse(value);
                        } else if (typeof value === 'object' && value !== null) {
                            oauth2Config = value;
                        }
                    } catch (e) {
                        console.warn('Failed to parse OAuth 2.0 config:', e);
                        oauth2Config = {};
                    }

                    // Also check for flattened OAuth2 fields in step.config
                    // Backend stores as: oauth2TokenUrl, oauth2ClientId, etc.
                    // OAuth2ConfigBuilder expects: tokenURL, clientID, etc.
                    if (step.config) {
                        if (step.config.oauth2TokenUrl) oauth2Config.tokenURL = step.config.oauth2TokenUrl;
                        if (step.config.oauth2ClientId) oauth2Config.clientID = step.config.oauth2ClientId;
                        if (step.config.oauth2ClientSecret) oauth2Config.clientSecret = step.config.oauth2ClientSecret;
                        if (step.config.oauth2GrantType) oauth2Config.grantType = step.config.oauth2GrantType;
                        if (step.config.oauth2Scope) oauth2Config.scope = step.config.oauth2Scope;
                        if (step.config.oauth2Audience) oauth2Config.audience = step.config.oauth2Audience;
                        if (step.config.oauth2Username) oauth2Config.username = step.config.oauth2Username;
                        if (step.config.oauth2Password) oauth2Config.password = step.config.oauth2Password;
                    }

                    // Create container for OAuth2ConfigBuilder component
                    html += `<div class="oauth2-config-builder-container" data-field-key="${field.key}" data-initial-config='${JSON.stringify(oauth2Config)}'></div>`;
                    break;
            }

            if (field.help) {
                html += `<small style="color: #6b7280; margin-top: 0.25rem; display: block;">${field.help}</small>`;
            }

            html += `</div>`;
        });

        html += `</div>`;

        return html;
    }

    /**
     * Create a single validation rule row
     * @param {Object} rule - The validation rule object
     * @param {number} index - The row index
     * @returns {string} HTML string for the rule row
     */
    createValidationRuleRow(rule, index) {
        const fieldPath = rule.field || rule.hl7Field || '';
        const isRequired = rule.required === true || rule.required === 'true';
        const validationType = rule.type || 'required';
        const errorMessage = rule.errorMessage || '';

        return `
            <div class="validation-rule-row" data-index="${index}">
                <div class="rule-fields">
                    <div class="rule-field">
                        <label>HL7 Field Path</label>
                        <input type="text"
                               class="rule-field-path"
                               placeholder="e.g., PID.5, MSH.9, OBX.5"
                               value="${fieldPath}"
                               data-rule-prop="field">
                        <small style="color: #6b7280;">Examples: PID.5 (Patient Name), MSH.9 (Message Type)</small>
                    </div>

                    <div class="rule-field">
                        <label>Validation Type</label>
                        <select class="rule-type" data-rule-prop="type">
                            <option value="required" ${validationType === 'required' ? 'selected' : ''}>Required Field</option>
                            <option value="format" ${validationType === 'format' ? 'selected' : ''}>Format Check</option>
                            <option value="length" ${validationType === 'length' ? 'selected' : ''}>Length Validation</option>
                            <option value="range" ${validationType === 'range' ? 'selected' : ''}>Value Range</option>
                            <option value="pattern" ${validationType === 'pattern' ? 'selected' : ''}>Regex Pattern</option>
                        </select>
                    </div>

                    <div class="rule-field">
                        <label>Error Message</label>
                        <input type="text"
                               class="rule-error-msg"
                               placeholder="Custom error message (optional)"
                               value="${errorMessage}"
                               data-rule-prop="errorMessage">
                    </div>

                    <div class="rule-actions">
                        <button type="button" class="btn-remove-rule" onclick="propertiesPanel.removeValidationRule(this)" title="Remove Rule">
                            <span style="color: #ef4444; font-size: 1.2em;">×</span>
                        </button>
                    </div>
                </div>
            </div>
        `;
    }

    /**
     * Add a new validation rule row
     * @param {HTMLElement} button - The "Add Rule" button that was clicked
     */
    addValidationRule(button) {
        const builderContainer = button.closest('.validation-builder');
        const rulesList = builderContainer.querySelector('.validation-rules-list');
        const currentRules = rulesList.querySelectorAll('.validation-rule-row');
        const newIndex = currentRules.length;

        // Create new row
        const newRow = this.createValidationRuleRow({}, newIndex);
        rulesList.insertAdjacentHTML('beforeend', newRow);

        // Update hidden field
        this.updateValidationRulesJSON(builderContainer);

        // Add event listeners to new row inputs
        this.attachValidationRuleListeners(builderContainer);
    }

    /**
     * Remove a validation rule row
     * @param {HTMLElement} button - The remove button that was clicked
     */
    removeValidationRule(button) {
        const ruleRow = button.closest('.validation-rule-row');
        const builderContainer = button.closest('.validation-builder');
        const rulesList = builderContainer.querySelector('.validation-rules-list');

        // Don't remove if it's the last row - just clear it
        const totalRows = rulesList.querySelectorAll('.validation-rule-row').length;
        if (totalRows === 1) {
            // Clear all inputs in the row
            ruleRow.querySelectorAll('input').forEach(input => input.value = '');
            ruleRow.querySelector('select').selectedIndex = 0;
        } else {
            // Remove the row
            ruleRow.remove();
        }

        // Update hidden field
        this.updateValidationRulesJSON(builderContainer);
    }

    /**
     * Update the hidden JSON field with current rule values
     * @param {HTMLElement} builderContainer - The validation-builder container
     */
    updateValidationRulesJSON(builderContainer) {
        const ruleRows = builderContainer.querySelectorAll('.validation-rule-row');
        const rules = [];

        ruleRows.forEach(row => {
            const fieldPath = row.querySelector('.rule-field-path').value.trim();
            const validationType = row.querySelector('.rule-type').value;
            const errorMessage = row.querySelector('.rule-error-msg').value.trim();

            // Only add non-empty rules
            if (fieldPath) {
                const rule = {
                    field: fieldPath,
                    type: validationType,
                    required: validationType === 'required'
                };

                if (errorMessage) {
                    rule.errorMessage = errorMessage;
                }

                rules.push(rule);
            }
        });

        // Update hidden field
        const hiddenField = builderContainer.querySelector('input[type="hidden"]');
        if (hiddenField) {
            hiddenField.value = JSON.stringify(rules);
        }
    }

    /**
     * Attach event listeners to validation rule inputs
     * @param {HTMLElement} builderContainer - The validation-builder container
     */
    attachValidationRuleListeners(builderContainer) {
        const inputs = builderContainer.querySelectorAll('.rule-field-path, .rule-type, .rule-error-msg');

        inputs.forEach(input => {
            // Remove existing listener to avoid duplicates
            input.removeEventListener('change', this._ruleChangeHandler);
            input.removeEventListener('input', this._ruleChangeHandler);

            // Add listener
            const handler = () => this.updateValidationRulesJSON(builderContainer);
            input.addEventListener('change', handler);
            input.addEventListener('input', handler);
        });
    }

    /**
     * Get step-specific configuration fields
     */
    getStepConfiguration(stepType) {
        const configurations = {
            'pre.validation': {
                fields: [
                    {
                        key: 'rules',
                        label: 'Validation Rules',
                        type: 'validation-builder',
                        required: true,
                        help: 'Add validation rules for HL7 fields. Use step-level controls (Required + Error Strategy) to control ACK/NACK behavior.'
                    },
                    {
                        key: 'detailedOutput',
                        label: 'Output Detail Level',
                        type: 'checkbox',
                        default: false,
                        checkboxLabel: 'Show detailed field-by-field validation results',
                        help: 'When enabled, step output includes detailed validation results for each field. When disabled, shows only summary (fields validated count and status).'
                    }
                ]
            },
            // REMOVED: 'pre.enrichment.metadata' configuration - now merged with Field Mapping (core.transformation)
            'pre.enrichment.api': {
                fields: [
                    {
                        key: 'endpoint',
                        label: 'API Endpoint',
                        type: 'text',
                        required: true,
                        placeholder: 'https://empi.hospital.org/api/patients/{patientId}',
                        help: 'API endpoint URL. Use {placeholder} for field values from HL7 message'
                    },
                    {
                        key: 'method',
                        label: 'HTTP Method',
                        type: 'select',
                        required: true,
                        default: 'GET',
                        options: [
                            { value: 'GET', label: 'GET' },
                            { value: 'POST', label: 'POST' },
                            { value: 'PUT', label: 'PUT' },
                            { value: 'PATCH', label: 'PATCH' }
                        ],
                        help: 'HTTP method for the API request'
                    },
                    {
                        key: 'authType',
                        label: 'Authentication Type',
                        type: 'select',
                        default: 'none',
                        options: [
                            { value: 'none', label: 'None' },
                            { value: 'basic', label: 'Basic Auth' },
                            { value: 'bearer', label: 'Bearer Token' },
                            { value: 'apikey', label: 'API Key' },
                            { value: 'oauth2', label: 'OAuth 2.0' }
                        ],
                        help: 'Authentication method for the API'
                    },
                    {
                        key: 'basicAuth',
                        label: 'Basic Authentication',
                        type: 'basic-auth-container',
                        required: false,
                        help: 'Username and password authentication',
                        visibleWhen: { field: 'authType', value: 'basic' }
                    },
                    {
                        key: 'bearerToken',
                        label: 'Bearer Token Authentication',
                        type: 'bearer-token-container',
                        required: false,
                        help: 'Token-based authentication',
                        visibleWhen: { field: 'authType', value: 'bearer' }
                    },
                    {
                        key: 'apiKeyAuth',
                        label: 'API Key Authentication',
                        type: 'apikey-container',
                        required: false,
                        help: 'API key authentication',
                        visibleWhen: { field: 'authType', value: 'apikey' }
                    },
                    {
                        key: 'oauth2Config',
                        label: 'OAuth 2.0 Configuration',
                        type: 'oauth2-builder',
                        required: false,
                        help: 'Configure OAuth 2.0 authentication (Client Credentials, Password Grant, etc.)',
                        visibleWhen: { field: 'authType', value: 'oauth2' }
                    },
                    {
                        key: 'headers',
                        label: 'HTTP Headers',
                        type: 'header-builder',
                        required: false,
                        help: 'Add custom HTTP headers for the API request'
                    },
                    {
                        key: 'queryParams',
                        label: 'Query Parameters',
                        type: 'query-param-builder',
                        required: false,
                        help: 'Add query parameters to the API URL'
                    },
                    {
                        key: 'fieldMappings',
                        label: 'Field Mappings (JSON)',
                        type: 'textarea',
                        rows: 4,
                        placeholder: '{"patientId": "PID.3", "messageType": "MSH.9"}',
                        help: 'Map placeholder names to HL7 field paths. Example: {"patientId": "PID.3"}'
                    },
                    {
                        key: 'targetPath',
                        label: 'Target Path',
                        type: 'text',
                        default: 'enriched.api',
                        placeholder: 'enriched.api',
                        help: 'Where to store API response in message data (dot notation)'
                    },
                    {
                        key: 'timeoutMs',
                        label: 'Timeout (ms)',
                        type: 'number',
                        default: 5000,
                        min: 100,
                        max: 30000,
                        help: 'API request timeout in milliseconds'
                    },
                    {
                        key: 'retryCount',
                        label: 'Retry Count',
                        type: 'number',
                        default: 0,
                        min: 0,
                        max: 5,
                        help: 'Number of retry attempts on failure (before applying Error Strategy)'
                    },
                    {
                        key: 'apiEndpointTester',
                        label: '🧪 Test API Endpoint',
                        type: 'api-endpoint-tester',
                        required: false,
                        help: 'Test your API configuration and see the actual response before configuring field mappings. NO-CODE: Click fields to automatically add them to response mapping.'
                    },
                    {
                        key: 'responseMapping',
                        label: '📤 Response Mapping (Output Variables)',
                        type: 'api-response-mapping-builder',
                        required: false,
                        help: 'Configure which fields from the API response to extract as step output variables. These variables can be referenced by subsequent steps.'
                    }
                    // Note: Error handling is controlled by step-level "On Error Strategy" setting:
                    // - "Fail" = Stop pipeline on API error
                    // - "Skip" = Continue without enrichment data
                    // - "Use Default Value" = Continue with defaultValue (if configured)
                ]
            },
            'pre.enrichment.database': {
                fields: [
                    {
                        key: 'databaseType',
                        label: 'Database Type',
                        type: 'select',
                        required: true,
                        options: [
                            { value: 'postgresql', label: 'PostgreSQL' },
                            { value: 'mysql', label: 'MySQL' },
                            { value: 'sqlserver', label: 'SQL Server' },
                            { value: 'oracle', label: 'Oracle' },
                            { value: 'mongodb', label: 'MongoDB' },
                            { value: 'redis', label: 'Redis' }
                        ],
                        help: 'Type of database to query (SQL and NoSQL)'
                    },
                    {
                        key: 'dbHost',
                        label: 'Host',
                        type: 'text',
                        required: true,
                        default: 'postgres',
                        placeholder: 'localhost or postgres',
                        help: 'Database server hostname or IP address'
                    },
                    {
                        key: 'dbPort',
                        label: 'Port',
                        type: 'number',
                        required: true,
                        default: 5432,
                        placeholder: 'PostgreSQL: 5432, MySQL: 3306, MongoDB: 27017, Redis: 6379, SQL Server: 1433, Oracle: 1521',
                        help: 'Database server port - changes based on database type'
                    },
                    {
                        key: 'dbName',
                        label: 'Database',
                        type: 'text',
                        required: true,
                        default: 'ezhealthkonnect',
                        placeholder: 'database_name or Redis DB number (0-15)',
                        help: 'Database name to connect to. For Redis, use database number 0-15 (default: 0)'
                    },
                    {
                        key: 'dbUser',
                        label: 'Username',
                        type: 'text',
                        required: false,
                        default: 'ezhealth_user',
                        placeholder: 'username',
                        help: 'Database username (not required for Redis)',
                        hideIf: { field: 'databaseType', values: ['redis'] }
                    },
                    {
                        key: 'dbPassword',
                        label: 'Password',
                        type: 'password',
                        required: true,
                        placeholder: 'password',
                        help: 'Database password (stored securely)'
                    },
                    {
                        key: 'collection',
                        label: 'Collection Name',
                        type: 'text',
                        required: false,
                        placeholder: 'patients',
                        help: 'MongoDB collection name (only for MongoDB)',
                        showIf: { field: 'databaseType', values: ['mongodb'] }
                    },
                    {
                        key: 'mongodbQueryMode',
                        label: 'Query Mode',
                        type: 'select',
                        required: false,
                        default: 'visual',
                        options: [
                            { value: 'visual', label: '👁️ User-Friendly (Visual Builder - Recommended for beginners)' },
                            { value: 'advanced', label: '⚡ Advanced (Aggregation Pipeline - For MongoDB experts)' }
                        ],
                        help: 'Choose how to build your MongoDB query. Visual mode = no coding required. Advanced mode = full MongoDB aggregation power.',
                        showIf: { field: 'databaseType', values: ['mongodb'] }
                    },
                    {
                        key: 'mongodbFilterBuilder',
                        label: 'Filter Conditions',
                        type: 'mongodb-filter-builder',
                        required: false,
                        help: 'Visual builder for MongoDB query filter. NO-CODE: Click to add conditions instead of writing JSON.',
                        showIf: { field: 'databaseType', values: ['mongodb'] },
                        visibleWhen: { field: 'mongodbQueryMode', value: 'visual' }
                    },
                    {
                        key: 'mongodbProjectionBuilder',
                        label: 'Select Fields',
                        type: 'mongodb-projection-builder',
                        required: false,
                        help: 'Visual field selector for MongoDB projection. NO-CODE: Check fields to include/exclude.',
                        showIf: { field: 'databaseType', values: ['mongodb'] },
                        visibleWhen: { field: 'mongodbQueryMode', value: 'visual' }
                    },
                    {
                        key: 'aggregationPipeline',
                        label: 'Aggregation Pipeline (JSON)',
                        type: 'textarea',
                        required: false,
                        rows: 15,
                        placeholder: '[\n  { "$match": { "mrn": "{PID.3}" } },\n  { "$project": { "fullName": { "$concat": ["$firstName", " ", "$lastName"] } } }\n]',
                        help: 'MongoDB aggregation pipeline as JSON array. Supports full MongoDB syntax including $concat, $group, $lookup, etc. Use {PID.3} for HL7 field placeholders.',
                        showIf: { field: 'databaseType', values: ['mongodb'] },
                        visibleWhen: { field: 'mongodbQueryMode', value: 'advanced' }
                    },
                    {
                        key: 'redisQueryBuilder',
                        label: '🔧 Redis Query (NO CODE)',
                        type: 'redis-query-builder',
                        required: false,
                        help: 'Build Redis query using visual form - no syntax knowledge needed! Select command, build key pattern, and preview the final command.',
                        showIf: { field: 'databaseType', values: ['redis'] }
                    },
                    {
                        key: 'query',
                        label: 'SQL Query',
                        type: 'textarea',
                        required: false,
                        rows: 4,
                        placeholder: 'SELECT * FROM patients WHERE patient_id = $1',
                        help: 'SQL query with parameter placeholders ($1, $2, etc.)',
                        showIf: { field: 'databaseType', values: ['postgresql', 'mysql', 'sqlserver', 'oracle'] }
                    },
                    {
                        key: 'queryParamsBuilder',
                        label: 'Query Parameters',
                        type: 'query-param-builder',
                        required: false,
                        help: 'Map SQL parameter placeholders to HL7 field paths. NO-CODE: Visual builder eliminates JSON editing.',
                        showIf: { field: 'databaseType', values: ['postgresql', 'mysql', 'sqlserver', 'oracle'] }
                    },
                    {
                        key: 'databaseQueryTester',
                        label: '🧪 Test Database Query',
                        type: 'database-query-tester',
                        required: false,
                        help: 'Test your SQL query with sample parameters before saving. NO-CODE: Click [+ Add to Mapping] on result fields to automatically add them to Result Mapping.',
                        showIf: { field: 'databaseType', values: ['postgresql', 'mysql', 'sqlserver', 'oracle'] }
                    },
                    {
                        key: 'resultMappingBuilder',
                        label: 'Result Mapping',
                        type: 'result-mapping-builder',
                        required: false,
                        help: 'Map database columns to output field names. NO-CODE: Use visual builder or click [+ Add to Mapping] from Query Tester results.',
                        showIf: { field: 'databaseType', values: ['postgresql', 'mysql', 'sqlserver', 'oracle'] }
                    },
                    {
                        key: 'targetPath',
                        label: 'Target Path',
                        type: 'text',
                        default: 'enriched.database',
                        placeholder: 'enriched.database',
                        help: 'Where to store query results in message data'
                    },
                    {
                        key: 'timeoutMs',
                        label: 'Timeout (ms)',
                        type: 'number',
                        default: 3000,
                        min: 100,
                        max: 30000,
                        help: 'Query timeout in milliseconds'
                    },
                    {
                        key: 'failOnError',
                        label: 'Fail on Error',
                        type: 'checkbox',
                        default: false,
                        checkboxLabel: 'Stop pipeline if query fails',
                        help: 'If unchecked, pipeline continues even if query fails'
                    }
                ]
            },
            'pre.enrichment.cache': {
                fields: [
                    {
                        key: 'cacheType',
                        label: 'Cache Type',
                        type: 'select',
                        required: true,
                        options: [
                            { value: 'redis', label: 'Redis' },
                            { value: 'memcached', label: 'Memcached' }
                        ],
                        help: 'Type of cache system to use'
                    },
                    {
                        key: 'connectionString',
                        label: 'Connection String',
                        type: 'text',
                        required: true,
                        placeholder: 'redis://localhost:6379',
                        help: 'Cache server connection string'
                    },
                    {
                        key: 'keyTemplate',
                        label: 'Cache Key Template',
                        type: 'text',
                        required: true,
                        placeholder: 'patient:{patientId}',
                        help: 'Key template with placeholders. Example: patient:{patientId}'
                    },
                    {
                        key: 'keyMappings',
                        label: 'Key Mappings (JSON)',
                        type: 'textarea',
                        rows: 3,
                        placeholder: '{"patientId": "PID.3"}',
                        help: 'Map placeholder names to HL7 field paths. Example: {"patientId": "PID.3"}'
                    },
                    {
                        key: 'targetPath',
                        label: 'Target Path',
                        type: 'text',
                        default: 'enriched.cache',
                        placeholder: 'enriched.cache',
                        help: 'Where to store cached data in message'
                    },
                    {
                        key: 'timeoutMs',
                        label: 'Timeout (ms)',
                        type: 'number',
                        default: 1000,
                        min: 100,
                        max: 10000,
                        help: 'Cache lookup timeout in milliseconds'
                    },
                    {
                        key: 'writeBack',
                        label: 'Write Back to Cache',
                        type: 'checkbox',
                        default: false,
                        checkboxLabel: 'Write enriched data back to cache',
                        help: 'If checked, enriched data will be written back to cache'
                    },
                    {
                        key: 'ttlSeconds',
                        label: 'TTL (seconds)',
                        type: 'number',
                        default: 3600,
                        min: 60,
                        max: 86400,
                        help: 'Time-to-live for cache entries (if write-back enabled)'
                    },
                    {
                        key: 'failOnError',
                        label: 'Fail on Error',
                        type: 'checkbox',
                        default: false,
                        checkboxLabel: 'Stop pipeline if cache lookup fails',
                        help: 'If unchecked, pipeline continues even if cache lookup fails'
                    }
                ]
            },
            'pre.enrichment.script': {
                fields: [
                    {
                        key: 'script',
                        label: 'JavaScript Code',
                        type: 'textarea',
                        required: true,
                        rows: 15,
                        // NO placeholder - start with completely blank textarea
                        help: `<div style="background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; padding: 12px; border-radius: 8px; margin-bottom: 8px; font-size: 13px;">
    <strong>📘 Available Functions:</strong><br>
    • <code style="background: rgba(255,255,255,0.2); padding: 2px 6px; border-radius: 3px;">getNestedValue(input, "path.to.field")</code> - Access HL7 fields or enriched data<br>
    • <code style="background: rgba(255,255,255,0.2); padding: 2px 6px; border-radius: 3px;">calculateAge(hl7Date)</code> - Calculate age from HL7 date (YYYYMMDD)<br>
    • <code style="background: rgba(255,255,255,0.2); padding: 2px 6px; border-radius: 3px;">parseHL7Date(hl7Date)</code> - Convert HL7 date to JavaScript Date<br>
    • <code style="background: rgba(255,255,255,0.2); padding: 2px 6px; border-radius: 3px;">console.log(message)</code> - Debug logging (check container logs)
</div>
<div style="background: #ecfdf5; border-left: 4px solid #059669; padding: 10px; border-radius: 4px; margin-top: 8px; font-size: 13px;">
    <strong style="color: #059669;">💡 Data Access Patterns:</strong><br>
    • <strong>HL7 Fields:</strong> <code style="background: #d1fae5; padding: 2px 6px; border-radius: 3px; color: #065f46;">enhancedSegments.PID.fields.5.value</code><br>
    • <strong>Metadata:</strong> <code style="background: #d1fae5; padding: 2px 6px; border-radius: 3px; color: #065f46;">metadata.yourKey</code> (from Metadata Enrichment)<br>
    • <strong>Database:</strong> <code style="background: #d1fae5; padding: 2px 6px; border-radius: 3px; color: #065f46;">enriched.database</code> (from Database Enrichment)<br>
    • <strong>Previous Script:</strong> <code style="background: #d1fae5; padding: 2px 6px; border-radius: 3px; color: #065f46;">enriched.script</code> (from earlier Script step)
</div>`
                    },
                    {
                        key: 'targetPath',
                        label: 'Target Path',
                        type: 'text',
                        default: 'enriched.script',
                        placeholder: 'enriched.script',
                        help: `<div style="background: #fef3c7; border-left: 4px solid #f59e0b; padding: 10px; border-radius: 4px; font-size: 13px;">
    <strong style="color: #d97706;">💾 Storage Path:</strong> Where to store script output in the message<br>
    <strong style="color: #92400e;">Examples:</strong><br>
    • <code style="background: #fde68a; padding: 2px 6px; border-radius: 3px;">enriched.script</code> - Default location<br>
    • <code style="background: #fde68a; padding: 2px 6px; border-radius: 3px;">enriched.risk.score</code> - Custom nested path<br>
    • <code style="background: #fde68a; padding: 2px 6px; border-radius: 3px;">enriched.patient.demographics</code> - Organized structure<br>
    <em style="color: #78350f;">💡 Use different paths for multiple script steps to avoid overwriting</em>
</div>`
                    },
                    {
                        key: 'timeoutMs',
                        label: 'Timeout (ms)',
                        type: 'number',
                        default: 5000,
                        min: 100,
                        max: 30000,
                        help: 'Script execution timeout in milliseconds'
                    },
                    {
                        key: 'failOnError',
                        label: 'Fail on Error',
                        type: 'checkbox',
                        default: false,
                        checkboxLabel: 'Stop pipeline if script fails',
                        help: 'If unchecked, pipeline continues even if script execution fails'
                    }
                ]
            },
            // Alias for new type name (layer prefix removed)
            'enrichment.script': null, // Will be aliased below
            'pre.enrichment': {
                fields: [
                    {
                        key: 'sources',
                        label: 'Data Sources',
                        type: 'multiselect',
                        required: true,
                        options: [
                            { value: 'EMPI', label: 'EMPI (Enterprise Master Patient Index)' },
                            { value: 'EHR', label: 'EHR System' },
                            { value: 'LIMS', label: 'Laboratory Information System' },
                            { value: 'RIS', label: 'Radiology Information System' },
                            { value: 'Custom', label: 'Custom API' }
                        ],
                        help: 'Select which external sources to query for patient enrichment'
                    },
                    {
                        key: 'timeout_ms',
                        label: 'Timeout (milliseconds)',
                        type: 'number',
                        default: 3000,
                        min: 100,
                        max: 30000,
                        step: 100,
                        help: 'Maximum time to wait for enrichment response'
                    },
                    {
                        key: 'failOnError',
                        label: 'Fail on Error',
                        type: 'checkbox',
                        default: false,
                        checkboxLabel: 'Stop pipeline if enrichment fails',
                        help: 'If checked, pipeline will fail if enrichment data cannot be retrieved'
                    }
                ]
            },
            'core.mapping': {
                fields: [
                    {
                        key: 'fhir_version',
                        label: 'FHIR Version',
                        type: 'select',
                        required: true,
                        options: [
                            { value: 'R4', label: 'FHIR R4' },
                            { value: 'R5', label: 'FHIR R5' },
                            { value: 'STU3', label: 'FHIR STU3' }
                        ],
                        default: 'R4',
                        help: 'Target FHIR version for transformation'
                    },
                    {
                        key: 'use_template',
                        label: 'Use Wizard Template',
                        type: 'checkbox',
                        default: true,
                        checkboxLabel: 'Use mappings from wizard configuration',
                        help: 'If checked, will use mappings configured in the wizard. Uncheck to manually configure mappings.'
                    }
                ]
            },
            'post.validation': {
                // FHIR validation now uses dedicated createFHIRValidationUI() method
                fields: []
            },
            'post.delivery': {
                fields: [
                    {
                        key: 'endpoint',
                        label: 'FHIR Server Endpoint',
                        type: 'text',
                        required: true,
                        placeholder: 'http://fhir-server:8080/fhir',
                        help: 'Full URL to the FHIR server endpoint'
                    },
                    {
                        key: 'resource',
                        label: 'Resource Type',
                        type: 'select',
                        required: true,
                        options: [
                            { value: 'Patient', label: 'Patient' },
                            { value: 'Encounter', label: 'Encounter' },
                            { value: 'Observation', label: 'Observation' },
                            { value: 'Bundle', label: 'Bundle (multiple resources)' }
                        ],
                        default: 'Bundle'
                    },
                    {
                        key: 'retry_count',
                        label: 'Retry Count',
                        type: 'number',
                        default: 3,
                        min: 0,
                        max: 10,
                        help: 'Number of times to retry failed deliveries'
                    }
                ]
            },
            'file_parser': {
                fields: [
                    {
                        key: 'sourceField',
                        label: 'Source Field',
                        type: 'text',
                        required: true,
                        placeholder: 'enriched.connector_result.content',
                        help: 'Field containing raw file content (e.g., from an inbound connector)'
                    },
                    {
                        key: 'fileFormat',
                        label: 'File Format',
                        type: 'select',
                        required: true,
                        options: [
                            { value: 'csv', label: 'CSV (Comma Separated)' },
                            { value: 'tsv', label: 'TSV (Tab Separated)' },
                            { value: 'fixed_width', label: 'Fixed Width / Positional (CCLF)' },
                            { value: 'xlsx', label: 'Excel (.xlsx)' }
                        ],
                        help: 'Format of the file to parse'
                    },
                    {
                        key: 'delimiter',
                        label: 'Delimiter',
                        type: 'text',
                        default: ',',
                        placeholder: ',',
                        help: 'Field delimiter character (default: comma for CSV, tab for TSV)'
                    },
                    {
                        key: 'hasHeader',
                        label: 'Has Header Row',
                        type: 'checkbox',
                        default: true,
                        checkboxLabel: 'First row contains column names'
                    },
                    {
                        key: 'sheetName',
                        label: 'Sheet Name (xlsx)',
                        type: 'text',
                        placeholder: '(default: first sheet)',
                        help: 'Which Excel sheet to parse (leave empty for first sheet)'
                    },
                    {
                        key: 'trimFields',
                        label: 'Trim Whitespace',
                        type: 'checkbox',
                        default: true,
                        checkboxLabel: 'Trim leading/trailing whitespace from values'
                    },
                    {
                        key: 'skipRows',
                        label: 'Skip Rows',
                        type: 'number',
                        default: 0,
                        min: 0,
                        help: 'Number of rows to skip from the top'
                    },
                    {
                        key: 'maxRecords',
                        label: 'Max Records',
                        type: 'number',
                        default: 0,
                        min: 0,
                        help: 'Maximum records to parse (0 = unlimited)'
                    }
                ]
            },
        };

        // Add aliases for renamed step types (layer prefix removed)
        configurations['enrichment.script'] = configurations['pre.enrichment.script'];
        configurations['enrichment.api'] = configurations['pre.enrichment.api'];
        configurations['enrichment.database'] = configurations['pre.enrichment.database'];
        configurations['field_validation'] = configurations['pre.validation'];
        configurations['hl7_fhir_transform'] = configurations['core.mapping'];
        configurations['field_mapping'] = configurations['core.transformation'];
        configurations['fhir_validation'] = configurations['post.validation'];

        return configurations[stepType] || null;
    }

    /**
     * Get step documentation
     */
    getStepDocumentation(stepType) {
        const docs = {
            'pre.validation': {
                description: 'Validates incoming HL7 messages against defined rules. Use step-level controls (Required + Error Strategy) to control whether validation failures stop the pipeline (NACK) or continue with warnings (ACK).',
                useCases: [
                    'Critical validation (Required=true): Patient safety fields → NACK on failure, stop pipeline',
                    'Data quality monitoring (Required=false + Error Strategy=continue): Accept with warnings → ACK, continue pipeline',
                    'Skip validation (Enabled=false): Emergency bypass → Skip all checks',
                    'Validate field formats (dates, numeric values, coded values)',
                    'Enforce business rules (age ranges, allowed codes, required fields)'
                ],
                example: {
                    rules: [
                        { field: 'enhancedSegments.PID.fields[0].value', type: 'required', errorMessage: 'Patient ID is required' },
                        { field: 'enhancedSegments.PID.fields[2].value', type: 'format', pattern: '^\\d{8}$', errorMessage: 'Birth date must be YYYYMMDD format' },
                        { field: 'enhancedSegments.PID.fields[1].subfields[0].value', type: 'required', errorMessage: 'Family name is required' },
                        { field: 'enhancedSegments.PV1.fields[1].value', type: 'enum', allowedValues: ['I', 'O', 'E'], errorMessage: 'Patient class must be I, O, or E' }
                    ]
                },
                parameters: [
                    {
                        name: 'field',
                        type: 'string',
                        required: true,
                        description: 'JSONPath to the field in parsed HL7 message. Use field selector autocomplete to choose from available fields. Examples: "enhancedSegments.PID.fields[0].value" for Patient ID, "enhancedSegments.PID.fields[1].subfields[0].value" for Family Name (atomic subfield).'
                    },
                    {
                        name: 'type',
                        type: 'enum',
                        required: true,
                        description: 'Validation type to apply. Available types: "required" (field must have a value), "format" (value must match regex pattern), "range" (numeric value within min/max), "length" (string length constraints), "enum" (value must be in allowed list), "date" (valid date format), "custom" (custom JavaScript validation).'
                    },
                    {
                        name: 'errorMessage',
                        type: 'string',
                        required: true,
                        description: 'Human-readable error message displayed when validation fails. Auto-populated based on field and validation type, but can be customized. Example: "Patient ID is required" or "Birth date must be YYYYMMDD format".'
                    },
                    {
                        name: 'pattern',
                        type: 'string (regex)',
                        required: false,
                        description: 'Regular expression pattern for "format" validation type. Example: "^\\d{8}$" for 8-digit date, "^[A-Z]{2}\\d{5}$" for 2 letters + 5 digits.'
                    },
                    {
                        name: 'minLength / maxLength',
                        type: 'number',
                        required: false,
                        description: 'String length constraints for "length" validation type. Example: minLength=1, maxLength=100 for patient name.'
                    },
                    {
                        name: 'min / max',
                        type: 'number',
                        required: false,
                        description: 'Numeric value range for "range" validation type. Example: min=0, max=120 for patient age.'
                    },
                    {
                        name: 'allowedValues',
                        type: 'Array<string>',
                        required: false,
                        description: 'List of allowed values for "enum" validation type. Example: ["M", "F", "U"] for administrative sex.'
                    },
                    {
                        name: 'customScript',
                        type: 'string (JavaScript)',
                        required: false,
                        description: 'Custom JavaScript function for "custom" validation type. Function receives field value and returns true/false. Example: "function(value) { return new Date(value) > new Date(\'1900-01-01\'); }"'
                    }
                ],
                validationTypes: [
                    {
                        type: 'required',
                        description: 'Field must have a non-empty value',
                        example: { field: 'enhancedSegments.PID.fields[0].value', type: 'required', errorMessage: 'Patient ID is required' },
                        usedFor: 'Ensuring critical fields like Patient ID, Message Type, or Date of Birth are present'
                    },
                    {
                        type: 'format',
                        description: 'Field value must match a regular expression pattern',
                        example: { field: 'enhancedSegments.PID.fields[2].value', type: 'format', pattern: '^\\d{8}$', errorMessage: 'Birth date must be YYYYMMDD' },
                        usedFor: 'Validating date formats, ID patterns, phone numbers, postal codes'
                    },
                    {
                        type: 'length',
                        description: 'String length must be within specified min/max range',
                        example: { field: 'enhancedSegments.PID.fields[1].subfields[0].value', type: 'length', minLength: 1, maxLength: 50, errorMessage: 'Family name must be 1-50 characters' },
                        usedFor: 'Enforcing name length limits, comment field constraints'
                    },
                    {
                        type: 'range',
                        description: 'Numeric value must be within specified min/max range',
                        example: { field: 'enhancedSegments.OBX.fields[4].value', type: 'range', min: 0, max: 300, errorMessage: 'Glucose level must be 0-300' },
                        usedFor: 'Validating lab values, patient age, vital signs'
                    },
                    {
                        type: 'enum',
                        description: 'Field value must be one of the allowed values',
                        example: { field: 'enhancedSegments.PID.fields[3].value', type: 'enum', allowedValues: ['M', 'F', 'U', 'O'], errorMessage: 'Sex must be M, F, U, or O' },
                        usedFor: 'Validating coded values like gender, patient class, result status'
                    },
                    {
                        type: 'date',
                        description: 'Field must be a valid date in specified format',
                        example: { field: 'enhancedSegments.PID.fields[2].value', type: 'date', format: 'YYYYMMDD', errorMessage: 'Invalid birth date format' },
                        usedFor: 'Validating birth dates, admission dates, observation timestamps'
                    },
                    {
                        type: 'custom',
                        description: 'Custom JavaScript validation function',
                        example: { field: 'enhancedSegments.PID.fields[2].value', type: 'custom', customScript: 'function(value) { const age = (Date.now() - new Date(value)) / 31557600000; return age >= 0 && age <= 120; }', errorMessage: 'Patient age must be 0-120 years' },
                        usedFor: 'Complex business rules, cross-field validation, calculated validations'
                    }
                ],
                fieldExamples: [
                    { path: 'enhancedSegments.MSH.fields[1].value', description: 'Message Type (e.g., ADT^A01)', segment: 'MSH', field: 'MSH.9' },
                    { path: 'enhancedSegments.PID.fields[0].value', description: 'Patient ID', segment: 'PID', field: 'PID.3' },
                    { path: 'enhancedSegments.PID.fields[1].value', description: 'Patient Name (full)', segment: 'PID', field: 'PID.5' },
                    { path: 'enhancedSegments.PID.fields[1].subfields[0].value', description: 'Family Name (atomic)', segment: 'PID', field: 'PID.5.1' },
                    { path: 'enhancedSegments.PID.fields[1].subfields[1].value', description: 'Given Name (atomic)', segment: 'PID', field: 'PID.5.2' },
                    { path: 'enhancedSegments.PID.fields[2].value', description: 'Date of Birth', segment: 'PID', field: 'PID.7' },
                    { path: 'enhancedSegments.PID.fields[3].value', description: 'Administrative Sex', segment: 'PID', field: 'PID.8' },
                    { path: 'enhancedSegments.PV1.fields[1].value', description: 'Patient Class (I/O/E)', segment: 'PV1', field: 'PV1.2' },
                    { path: 'enhancedSegments.PV1.fields[10].value', description: 'Admission Date/Time', segment: 'PV1', field: 'PV1.44' }
                ]
            },
            'pre.enrichment.api': {
                description: 'Enriches HL7 messages by querying external REST APIs (EMPI, EHR, LIMS, insurance systems). Supports all authentication methods including OAuth 2.0 with automatic token management. 100% Postman feature parity.',
                useCases: [
                    'Epic FHIR EMPI lookup - Get complete patient demographics using MRN',
                    'Cerner EMPI - Retrieve patient master index data with OAuth 2.0',
                    'LIMS integration - Fetch pending lab orders for patient',
                    'Insurance verification - Check coverage and eligibility in real-time',
                    'Provider directory - Lookup NPI, specialty, DEA number from external API'
                ],
                example: {
                    endpoint: 'https://epic-fhir.hospital.org/api/FHIR/R4/Patient/{patientId}',
                    method: 'GET',
                    authType: 'oauth2',
                    oauth2Config: {
                        grantType: 'client_credentials',
                        tokenURL: 'https://epic-fhir.hospital.org/oauth2/token',
                        clientID: 'integration-engine',
                        clientSecret: '***',
                        scope: 'patient/*.read'
                    },
                    headers: {
                        'Accept': 'application/fhir+json',
                        'Epic-Client-ID': 'integration-engine'
                    },
                    queryParams: {
                        '_format': 'json',
                        '_pretty': 'true'
                    },
                    fieldMappings: {
                        patientId: 'enhancedSegments.PID.fields[2].value'
                    },
                    targetPath: 'enriched.empi',
                    timeoutMs: 5000,
                    retryCount: 2
                },
                parameters: [
                    { name: 'endpoint', type: 'string', required: true, description: 'API endpoint URL. Use {placeholder} for dynamic values from HL7 fields. Example: https://api.empi.org/patients/{patientId}' },
                    { name: 'method', type: 'enum (GET|POST|PUT|PATCH)', required: true, description: 'HTTP method for the API request' },
                    { name: 'authType', type: 'enum (none|basic|bearer|apikey|oauth2)', required: false, description: 'Authentication method: none (no auth), basic (username/password), bearer (token), apikey (API key in header), oauth2 (OAuth 2.0 with automatic token management)' },
                    { name: 'oauth2Config', type: 'object', required: false, description: 'OAuth 2.0 configuration - ONLY when authType=oauth2. Includes: grantType (client_credentials|password|refresh_token), tokenURL, clientID, clientSecret, scope. Automatic token caching and refresh.' },
                    { name: 'headers', type: 'object', required: false, description: 'HTTP headers as key-value pairs. Use HeaderBuilder UI for visual configuration. Example: {"Accept": "application/json", "Epic-Client-ID": "integration-engine"}' },
                    { name: 'queryParams', type: 'object', required: false, description: 'Query parameters as key-value pairs. Use QueryParamBuilder UI for visual configuration with live URL preview. Example: {"_format": "json", "_count": "10"}' },
                    { name: 'fieldMappings', type: 'object (JSON)', required: false, description: 'Maps placeholder names in URL to HL7 field paths. Example: {"patientId": "enhancedSegments.PID.fields[2].value"} replaces {patientId} in URL with PID-3 value' },
                    { name: 'targetPath', type: 'string', required: false, description: 'Where to store API response in message data using dot notation. Default: "enriched.api". Example: "enriched.empi" stores response at message.enriched.empi' },
                    { name: 'timeoutMs', type: 'number (100-30000)', required: false, description: 'API request timeout in milliseconds. Default: 5000 (5 seconds). Prevents hanging on slow APIs' },
                    { name: 'retryCount', type: 'number (0-5)', required: false, description: 'Number of retry attempts on failure before applying step-level Error Strategy. Default: 0. Uses exponential backoff for network resilience' },
                    { name: 'Error Handling', type: 'Step-level setting', required: false, description: 'Use "On Error Strategy" in Execution Settings to control pipeline behavior on API failure: "Fail" stops pipeline, "Skip" continues without data, "Use Default Value" continues with defaultValue (if configured in backend executor)' }
                ]
            },
            'pre.enrichment.database': {
                description: 'Enriches HL7 messages by querying databases (PostgreSQL, MySQL, SQL Server, MongoDB, Redis, Oracle). NO-CODE: Visual builders for query parameters and result mapping. Test queries before saving with real database results. Works even when query returns 0 rows - column autocomplete always available!',
                useCases: [
                    'EMPI lookup - Query patient master index database using MRN or SSN',
                    'Provider credentials - Fetch NPI, DEA, specialty from provider database',
                    'Patient demographics - Retrieve complete patient profile from EHR database',
                    'Lab reference ranges - Get normal ranges from LIMS database based on test code',
                    'Insurance verification - Check coverage in billing database using policy number',
                    'Facility master data - Lookup facility addresses, phone numbers, identifiers'
                ],
                databaseConfigs: {
                    mysql: {
                        name: '🐬 MySQL',
                        connectionFormat: 'username:password@tcp(host:port)/database',
                        example: 'app_user:mypassword@tcp(mysql.example.com:3306)/healthcare_db',
                        queryFormat: 'Use ? for parameter placeholders',
                        queryExample: 'SELECT * FROM patients WHERE mrn = ? AND dob = ?',
                        features: [
                            'Default port: 3306',
                            'Multi-statement queries supported',
                            'JSON functions (JSON_EXTRACT, JSON_VALUE)',
                            'Date formatting (DATE_FORMAT)',
                            'Use PID.3.1 (not PID.3) to match simple varchar columns'
                        ]
                    },
                    postgresql: {
                        name: '🐘 PostgreSQL',
                        connectionFormat: 'host=hostname port=port user=username password=password dbname=database sslmode=disable',
                        example: 'host=postgres.example.com port=5432 user=app_user password=mypassword dbname=healthcare_db sslmode=disable',
                        queryFormat: 'Use $1, $2, $3... for parameter placeholders',
                        queryExample: 'SELECT * FROM patients WHERE mrn = $1 AND dob = $2',
                        features: [
                            'Default port: 5432',
                            'Advanced JSON/JSONB support',
                            'Array operations',
                            'Window functions',
                            'SSL modes: disable, require, verify-ca, verify-full'
                        ]
                    },
                    sqlserver: {
                        name: '🏢 SQL Server',
                        connectionFormat: 'sqlserver://username:password@host:port?database=dbname',
                        example: 'sqlserver://app_user:mypassword@sqlserver.example.com:1433?database=HealthcareDB',
                        queryFormat: 'Use @p1, @p2, @p3... for parameter placeholders',
                        queryExample: 'SELECT * FROM patients WHERE mrn = @p1 AND DateOfBirth = @p2',
                        features: [
                            'Default port: 1433',
                            'Windows Authentication supported',
                            'TOP clause for limiting rows',
                            'JSON support (JSON_VALUE, FOR JSON PATH)',
                            'Common Table Expressions (CTEs)'
                        ]
                    },
                    mongodb: {
                        name: '🍃 MongoDB',
                        connectionFormat: 'mongodb://username:password@host:port/database?authSource=admin',
                        example: 'mongodb://app_user:mypassword@mongodb.example.com:27017/healthcare_db?authSource=admin',
                        queryFormat: 'Visual query builders for filter and projection (NO raw JSON required!)',
                        queryExample: '{ "mrn": "{{ PID.3.1 }}", "status": "active" }',
                        features: [
                            'Default port: 27017',
                            'NoSQL document database',
                            'Filter Builder for match conditions',
                            'Projection Builder for field selection',
                            'Advanced Mode for raw query editing',
                            'Nested document queries',
                            'Array operations ($in, $elemMatch)'
                        ]
                    },
                    redis: {
                        name: '⚡ Redis',
                        connectionFormat: 'redis://[:password@]host:port/database',
                        example: 'redis://:mypassword@redis.example.com:6379/0',
                        queryFormat: 'Redis commands (GET, HGETALL, SMEMBERS, etc.)',
                        queryExample: 'GET patient:{{ PID.3.1 }}',
                        features: [
                            'Default port: 6379',
                            'Key-value operations',
                            'Hash field retrieval (HGETALL)',
                            'Set operations (SMEMBERS)',
                            'Caching and temporary storage',
                            'Fast lookups for frequently accessed data'
                        ]
                    },
                    oracle: {
                        name: '🔴 Oracle',
                        connectionFormat: 'oracle://username:password@host:port/servicename',
                        example: 'oracle://app_user:mypassword@oracle.example.com:1521/ORCL',
                        queryFormat: 'Use :1, :2, :3... for parameter placeholders',
                        queryExample: 'SELECT * FROM patients WHERE mrn = :1 AND date_of_birth = :2',
                        features: [
                            'Default port: 1521',
                            'TNS Names format supported',
                            'ROWNUM for limiting rows',
                            'Hierarchical queries (CONNECT BY)',
                            'Advanced date functions (TO_CHAR)',
                            'Enterprise-grade features'
                        ]
                    }
                },
                example: {
                    databaseType: 'PostgreSQL',
                    connectionString: 'postgresql://user:pass@hostname:5432/database',
                    query: 'SELECT id, email, role, created_at FROM users WHERE email = $1',
                    queryParams: { '1': 'enhancedSegments.PID.fields[13].value' },
                    resultMapping: {
                        'id': 'userId',
                        'email': 'userEmail',
                        'role': 'userRole',
                        'created_at': 'userCreatedAt'
                    },
                    targetPath: 'enriched.user',
                    timeoutMs: 3000,
                    failOnError: false
                },
                parameters: [
                    {
                        name: 'databaseType',
                        type: 'enum (PostgreSQL|MySQL|SQL Server|MongoDB|Oracle)',
                        required: true,
                        description: 'Database type. Determines SQL driver and connection protocol.'
                    },
                    {
                        name: 'connectionString',
                        type: 'string',
                        required: true,
                        description: 'Database connection string. Format varies by database type. Examples: PostgreSQL: "postgresql://user:pass@host:5432/db", MySQL: "mysql://user:pass@host:3306/db", SQL Server: "sqlserver://user:pass@host:1433?database=db"'
                    },
                    {
                        name: 'query',
                        type: 'string (SQL)',
                        required: true,
                        description: 'SQL query to execute. Use $1, $2, $3... for parameter placeholders (PostgreSQL syntax). Use ? for MySQL/SQL Server. Test your query with the Query Tester before saving!'
                    },
                    {
                        name: 'queryParams (Visual Builder)',
                        type: 'object (Auto-built)',
                        required: false,
                        description: 'NO-CODE: Visual table maps SQL parameters to HL7 field paths. NO JSON EDITING REQUIRED! Example: $1 → enhancedSegments.PID.fields[13].value maps first parameter to PID-13 (Phone Number). Click [+ Add Parameter] to add rows.'
                    },
                    {
                        name: 'resultMapping (Visual Builder)',
                        type: 'object (Auto-built)',
                        required: false,
                        description: 'NO-CODE: Visual table maps database columns to output field names. NO JSON EDITING REQUIRED! Test your query first, then click [+ Add to Mapping] on result fields to auto-populate this builder. Auto converts snake_case to camelCase (created_at → createdAt).'
                    },
                    {
                        name: 'targetPath',
                        type: 'string',
                        required: false,
                        description: 'Where to store database results in message data using dot notation. Default: "enriched.database". Example: "enriched.empi" stores results at message.enriched.empi. Use different paths for multiple database enrichment steps.'
                    },
                    {
                        name: 'timeoutMs',
                        type: 'number (100-30000)',
                        required: false,
                        description: 'Database query timeout in milliseconds. Default: 3000 (3 seconds). Prevents hanging on slow queries or network issues.'
                    },
                    {
                        name: 'failOnError',
                        type: 'boolean',
                        required: false,
                        description: 'Whether to stop pipeline if database query fails. Default: false (continue without enrichment data). Set to true for critical enrichment that MUST succeed.'
                    }
                ],
                noCodeFeatures: [
                    {
                        feature: 'Query Parameter Builder',
                        description: 'Visual table for mapping SQL parameters ($1, $2, etc.) to HL7 field paths. NO JSON EDITING!',
                        howTo: 'Click [+ Add Parameter] → Select HL7 field from dropdown or type field path → Parameter automatically numbered',
                        benefit: 'Eliminates JSON syntax errors, provides field autocomplete, shows parameter order visually'
                    },
                    {
                        feature: 'Result Mapping Builder',
                        description: 'Visual table for mapping database columns to output field names. AUTO camelCase conversion!',
                        howTo: 'Click [+ Add Mapping] → Enter DB column name → Enter output field name (or use suggestion) → Auto-converts snake_case to camelCase',
                        benefit: 'No JSON editing, auto naming conventions, visual feedback on mappings'
                    },
                    {
                        feature: 'Database Query Tester',
                        description: 'Test SQL queries BEFORE saving pipeline! See real database results with click-to-add mapping.',
                        howTo: '1) Configure connection & query 2) Enter test parameter values 3) Click [▶ Run Query] 4) See actual results 5) Click [+ Add to Mapping] on any field',
                        benefit: 'Instant feedback, verify queries work, one-click configuration from real data, no trial-and-error'
                    },
                    {
                        feature: 'Click-to-Add Mapping',
                        description: 'Click [+ Add to Mapping] on query results to auto-populate Result Mapping Builder',
                        howTo: 'Run query → Click [+ Add to Mapping] next to any field → Automatically adds row to Result Mapping Builder with smart field name',
                        benefit: '80% time savings, zero typos, smart camelCase conversion (created_at → createdAt)'
                    }
                ],
                workflow: [
                    { step: 1, action: 'Select database type', description: 'Choose PostgreSQL, MySQL, SQL Server, MongoDB, or Oracle' },
                    { step: 2, action: 'Enter connection string', description: 'Database connection URL with credentials (secure storage recommended)' },
                    { step: 3, action: 'Write SQL query', description: 'Use $1, $2... for parameters. Example: SELECT * FROM patients WHERE mrn = $1' },
                    { step: 4, action: 'Map query parameters (Visual Builder)', description: 'Click [+ Add Parameter] → Select HL7 field path for each $1, $2, etc.' },
                    { step: 5, action: 'Test your query! 🧪', description: 'Scroll to Query Tester → Enter test values → Click [▶ Run Query] → See real results!' },
                    { step: 6, action: 'Click-to-add result mappings', description: 'Click [+ Add to Mapping] on fields you want to include → Auto-populates Result Mapping Builder' },
                    { step: 7, action: 'Configure target path', description: 'Set where to store results (e.g., enriched.empi, enriched.provider)' },
                    { step: 8, action: 'Save step', description: 'Configuration saved with visual builders + tested query' }
                ],
                bestPractices: [
                    {
                        practice: 'Always test queries first',
                        reason: 'Query Tester shows real results and prevents SQL errors in production',
                        example: 'Run test query with sample MRN before deploying pipeline'
                    },
                    {
                        practice: 'Use named connection strings (future)',
                        reason: 'Centralized connection management, credential rotation, environment-specific configs',
                        example: 'Instead of hardcoding connection string, reference named connection "EMPI_PROD"'
                    },
                    {
                        practice: 'Limit result rows in query',
                        reason: 'Prevents performance issues from large result sets',
                        example: 'Add LIMIT 1 to queries that should return single row (patient lookup, provider lookup)'
                    },
                    {
                        practice: 'Use specific column names',
                        reason: 'SELECT * causes issues when schema changes. Specify exact columns needed.',
                        example: 'Use SELECT id, name, mrn instead of SELECT *'
                    },
                    {
                        practice: 'Set appropriate timeout',
                        reason: 'Balance between allowing slow queries and preventing pipeline hangs',
                        example: 'Simple lookups: 1000ms, complex joins: 5000ms, reporting queries: 10000ms'
                    },
                    {
                        practice: 'Use failOnError wisely',
                        reason: 'Critical enrichment should fail pipeline; optional enrichment should continue',
                        example: 'EMPI lookup (critical): failOnError=true, Provider specialty (nice-to-have): failOnError=false'
                    }
                ],
                troubleshooting: [
                    {
                        issue: 'Connection failed: dial tcp: lookup failed',
                        cause: 'Hostname not resolvable or database server not reachable',
                        fix: 'Check connection string hostname. For Docker: use service name (e.g., postgres) not localhost'
                    },
                    {
                        issue: 'Query execution failed: syntax error',
                        cause: 'Invalid SQL syntax for selected database type',
                        fix: 'Test query in Query Tester. Check parameter syntax ($1 vs ? based on database type)'
                    },
                    {
                        issue: 'Query returned no rows',
                        cause: 'Query executed but no matching records found',
                        fix: 'Verify test parameter values match actual data. Check WHERE clause conditions.'
                    },
                    {
                        issue: 'Timeout exceeded',
                        cause: 'Query took longer than timeoutMs setting',
                        fix: 'Increase timeout, optimize query with indexes, or limit result set size'
                    },
                    {
                        issue: 'Parameter mismatch: expected 2 got 1',
                        cause: 'Query has more/fewer parameters than mapped in Query Parameter Builder',
                        fix: 'Count $1, $2, $3... in query. Add corresponding rows in Query Parameter Builder.'
                    }
                ],
                securityNotes: [
                    {
                        note: 'Secure credential storage',
                        detail: 'Connection strings contain passwords. Use environment variables or secret management systems (HashiCorp Vault, AWS Secrets Manager) instead of hardcoding.',
                        recommendation: 'Future feature: Named connections with credential vault integration'
                    },
                    {
                        note: 'SQL injection prevention',
                        detail: 'Always use parameterized queries ($1, $2...) - NEVER concatenate user input into SQL strings',
                        recommendation: 'Query Tester validates parameter usage. Backend uses prepared statements for safety.'
                    },
                    {
                        note: 'Least privilege database access',
                        detail: 'Database user should have SELECT-only permissions for read operations',
                        recommendation: 'Create dedicated integration user with minimal required permissions'
                    },
                    {
                        note: 'Connection pooling',
                        detail: 'Test queries use isolated connections with max 1 connection to prevent resource exhaustion',
                        recommendation: 'Production executor uses connection pooling for performance'
                    }
                ]
            },
            'pre.enrichment.script': {
                description: 'Execute custom JavaScript logic to calculate, transform, or enrich data from HL7 messages and previous enrichment steps. Use this for complex business rules, risk scoring, data validation, custom calculations, and conditional logic that cannot be achieved through simple mappings. Results are stored in enriched_data and can be referenced by subsequent steps.',
                useCases: [
                    'Calculate patient risk scores based on age, diagnoses, and lab values',
                    'Determine insurance eligibility by combining patient demographics with external data',
                    'Perform complex date calculations (e.g., days since last admission, appointment intervals)',
                    'Apply business rules for message routing or prioritization',
                    'Validate and transform data formats (e.g., normalize phone numbers, format addresses)',
                    'Enrich data by combining HL7 fields with database/API results from previous steps',
                    'Generate derived fields (e.g., BMI from height/weight, age from date of birth)',
                    'Implement conditional logic for data transformation (if-then-else scenarios)'
                ],
                example: {
                    script: `// Access HL7 message fields
var patientName = getHL7Field(input, "PID.5");
var dateOfBirth = getHL7Field(input, "PID.7");
var smokingStatus = getHL7Field(input, "PV1.17");

// Access enriched data from previous database enrichment step
var chronicConditions = getNestedValue(input, '["database_enrichment"].enriched_data.chronicConditions');
var lastAdmission = getNestedValue(input, '["database_enrichment"].enriched_data.lastAdmission');

// Access data from previous API enrichment step
var insuranceStatus = getNestedValue(input, '["API_Enrichment"].enriched_data.insuranceActive');

// Perform calculations
var patientAge = calculateAge(dateOfBirth);
var daysSinceLastAdmission = calculateDaysSince(lastAdmission);

// Calculate risk score based on multiple factors
var riskScore = 0;
if (patientAge > 65) riskScore += 3;
if (chronicConditions > 2) riskScore += 4;
if (smokingStatus === "current") riskScore += 2;
if (daysSinceLastAdmission < 30) riskScore += 3;

// Determine risk level
var riskLevel = "low";
if (riskScore >= 8) riskLevel = "high";
else if (riskScore >= 5) riskLevel = "moderate";

// Build risk factors array
var riskFactors = [];
if (chronicConditions > 2) riskFactors.push(chronicConditions + " chronic conditions");
if (smokingStatus === "current") riskFactors.push("Current smoker");
if (daysSinceLastAdmission < 30) riskFactors.push("Recent admission (< 30 days)");

// Return enriched data (stored in enriched_data for use in subsequent steps)
return {
    patientAge: patientAge,
    riskScore: riskScore,
    riskLevel: riskLevel,
    riskFactors: riskFactors,
    chronicConditions: chronicConditions,
    smokingStatus: smokingStatus,
    daysSinceLastAdmission: daysSinceLastAdmission,
    calculatedAt: new Date().toISOString()
};`,
                    timeout_ms: 5000,
                    failOnError: false
                },
                parameters: [
                    {
                        name: 'script',
                        type: 'string (JavaScript code)',
                        required: true,
                        description: 'JavaScript code to execute. The script receives the input object containing HL7 message and enriched data from previous steps. Must return an object with calculated/enriched fields.'
                    },
                    {
                        name: 'timeout_ms',
                        type: 'number',
                        required: false,
                        description: 'Maximum execution time in milliseconds (default: 5000). Script will be terminated if it exceeds this limit.'
                    },
                    {
                        name: 'failOnError',
                        type: 'boolean',
                        required: false,
                        description: 'Whether to fail the entire pipeline if script execution fails (default: false). Set to true for critical calculations.'
                    }
                ],
                referenceVariables: {
                    title: 'Accessing Data in Script Enrichment',
                    description: 'Your script can access HL7 message fields and enriched data from previous steps using helper functions. All returned fields are automatically available to subsequent steps.',
                    examples: [
                        {
                            scenario: 'Access HL7 Message Fields',
                            code: 'var patientId = getHL7Field(input, "PID.3");',
                            explanation: 'Use getHL7Field(input, "segment.field") to extract values from the HL7 message'
                        },
                        {
                            scenario: 'Access Database Enrichment Results',
                            code: 'var chronicConditions = getNestedValue(input, \'["database_enrichment"].enriched_data.chronicConditions\');',
                            explanation: 'Use getNestedValue(input, xpath) to access data from previous database enrichment steps'
                        },
                        {
                            scenario: 'Access API Enrichment Results',
                            code: 'var externalId = getNestedValue(input, \'["API_Enrichment"].enriched_data.externalPatientId\');',
                            explanation: 'Access data returned from API calls in previous enrichment steps'
                        },
                        {
                            scenario: 'Access Metadata Fields',
                            code: 'var customValue = getNestedValue(input, "metadata.customField");',
                            explanation: 'Access metadata fields added by the metadata enrichment step'
                        },
                        {
                            scenario: 'Return Calculated Values',
                            code: 'return { riskScore: 9, riskLevel: "moderate", calculatedAt: new Date().toISOString() };',
                            explanation: 'Return an object with calculated fields. These will be stored in ["Script_Enrichment"].enriched_data and available to subsequent steps including field mapping.'
                        }
                    ]
                },
                availableFunctions: {
                    title: 'Available Helper Functions',
                    description: 'The following functions are available in your script execution environment:',
                    functions: [
                        {
                            name: 'getHL7Field(input, path)',
                            description: 'Extract a field value from the HL7 message',
                            parameters: 'input: message object, path: HL7 field path (e.g., "PID.5", "PV1.3")',
                            returns: 'string',
                            example: 'var name = getHL7Field(input, "PID.5");'
                        },
                        {
                            name: 'getNestedValue(input, xpath)',
                            description: 'Access nested data from enriched_data fields using XPath notation',
                            parameters: 'input: message object, xpath: XPath to enriched field (e.g., \'["database_enrichment"].enriched_data.chronicConditions\')',
                            returns: 'any',
                            example: 'var conditions = getNestedValue(input, \'["database_enrichment"].enriched_data.chronicConditions\');'
                        },
                        {
                            name: 'calculateAge(dateOfBirth)',
                            description: 'Calculate age in years from date of birth',
                            parameters: 'dateOfBirth: date string in HL7 format (YYYYMMDD)',
                            returns: 'number',
                            example: 'var age = calculateAge("19800515");'
                        },
                        {
                            name: 'calculateDaysSince(dateString)',
                            description: 'Calculate number of days between a date and now',
                            parameters: 'dateString: date string in HL7 format (YYYYMMDD)',
                            returns: 'number',
                            example: 'var days = calculateDaysSince("20240101");'
                        },
                        {
                            name: 'formatDate(hl7Date, format)',
                            description: 'Convert HL7 date format to custom format',
                            parameters: 'hl7Date: HL7 date string, format: target format (e.g., "YYYY-MM-DD")',
                            returns: 'string',
                            example: 'var isoDate = formatDate("20240115", "YYYY-MM-DD");'
                        }
                    ]
                },
                bestPractices: [
                    {
                        practice: 'Error Handling',
                        recommendation: 'Always validate input data before performing calculations to avoid runtime errors',
                        example: 'if (chronicConditions && chronicConditions > 0) { /* safe to use */ }'
                    },
                    {
                        practice: 'Performance',
                        recommendation: 'Keep scripts lightweight and avoid complex loops. Use database/API enrichment for heavy data operations.',
                        example: 'Perform database queries in database enrichment step, use script enrichment only for calculations on retrieved data'
                    },
                    {
                        practice: 'Naming',
                        recommendation: 'Use clear, descriptive field names in your return object for easy reference in subsequent steps',
                        example: 'return { patientRiskScore: score } instead of return { r: score }'
                    },
                    {
                        practice: 'Testing',
                        recommendation: 'Use the Test Execution feature to verify script logic with sample HL7 messages before deployment',
                        example: 'Click "Test Execution" button and review the Script Enrichment output in step_outputs'
                    },
                    {
                        practice: 'Data Access',
                        recommendation: 'Click the "Variables" tab to see all available data from previous steps with copy-paste ready XPaths',
                        example: 'Copy XPath from Variables tab and use with getNestedValue() function'
                    }
                ]
            },
            'pre.enrichment': {
                description: 'Enriches HL7 messages with additional data from external systems (EMPI, EHR, etc.). Enhances message content before FHIR transformation.',
                useCases: [
                    'Add complete patient demographics from EMPI using partial identifier',
                    'Fetch latest lab results from LIMS',
                    'Retrieve insurance information from billing system',
                    'Augment provider data with NPI and specialty information',
                    'Add facility details (address, contact info) from master data management'
                ],
                example: {
                    sources: ['EMPI', 'EHR'],
                    timeout_ms: 3000,
                    failOnError: false,
                    caching: {
                        enabled: true,
                        ttl_seconds: 300
                    }
                },
                parameters: [
                    { name: 'sources', type: 'Array<string>', required: true, description: 'List of data sources to query (EMPI, EHR, LIMS, etc.)' },
                    { name: 'timeout_ms', type: 'number', required: false, description: 'Maximum time to wait for enrichment (default: 3000)' },
                    { name: 'failOnError', type: 'boolean', required: false, description: 'Whether to fail pipeline if enrichment fails (default: false)' }
                ]
            },
            // REMOVED: 'pre.enrichment.metadata' documentation - now merged with Field Mapping (core.transformation)
            'core.mapping': {
                description: 'Transforms HL7 v2 messages to FHIR R4 format in two passes:\n\n' +
                    '1. Field mapping — each row in the Visual Mapping tab maps one HL7 field (e.g. PID.5) to one FHIR path (e.g. Patient.name[0].family). ' +
                    'Enriched data from upstream steps can be referenced as source fields using the ["step_name"].enriched_data.field syntax.\n\n' +
                    '2. Structural assembly (Assembly tab) — runs automatically after field mapping for message types that need it. ' +
                    'For ORU^R01 this converts every OBX segment into a properly structured FHIR Observation (value type dispatch, ' +
                    'structured referenceRange, interpretation flags, laboratory category) and links them all to DiagnosticReport.result[]. ' +
                    'Each assembly rule can be individually disabled in the Assembly tab if your interface requires custom handling. ' +
                    'To extend beyond what the OOB rules produce, add a Script Enrichment step after this one — it receives the fully assembled fhirResources array.',
                useCases: [
                    'Convert ADT^A01 (patient admission) to FHIR Patient + Encounter + AllergyIntolerance',
                    'Transform ORU^R01 (lab results) to FHIR Observation + DiagnosticReport with OBX value dispatch',
                    'Map ORM^O01 (order) to FHIR ServiceRequest',
                    'Convert DFT^P03 (billing) to FHIR Claim',
                    'Enrich FHIR resources with lookup data from a prior Database Enrichment step',
                    'Disable specific assembly rules (e.g. referenceRange parsing) for non-standard interfaces',
                    'Add a Script Enrichment step after this one to post-process or extend assembled FHIR resources'
                ],
                example: {
                    fhir_version: 'R4',
                    use_template: true,
                    assembleObservations: true,
                    assemblyRules: {
                        obs_value_dispatch: true,
                        obs_reference_range: true,
                        obs_interpretation: true,
                        dr_result_links: true
                    },
                    mappings: [
                        {
                            comment: 'Patient family name from PID.5',
                            hl7Field: 'PID.5',
                            fhirPath: 'Patient.name[0].family',
                            dataType: 'string'
                        },
                        {
                            comment: 'Use enriched data from database lookup',
                            hl7Field: '["database_enrichment"].enriched_data.insuranceProvider',
                            fhirPath: 'Coverage.payor[0].display',
                            dataType: 'string'
                        }
                    ]
                },
                parameters: [
                    { name: 'fhir_version', type: 'string', required: true, description: 'Target FHIR version. Currently R4 is fully supported.' },
                    { name: 'use_template', type: 'boolean', required: false, description: 'Use wizard-configured field mappings (default: true).' },
                    { name: 'mappings', type: 'array', required: false, description: 'Array of field mappings. Each entry maps one HL7 field or enriched value to one FHIR path.' },
                    { name: 'assembleObservations', type: 'boolean', required: false, description: 'Master switch for structural assembly. When false all assembly rules are skipped regardless of individual rule settings. Default: true.' },
                    { name: 'assemblyRules', type: 'object', required: false, description: 'Per-rule on/off flags for structural assembly. All rules default to true when absent. Set a key to false to disable that specific transform. Keys: collapse_text_obx, obs_value_dispatch, obs_value_unit, obs_code, obs_reference_range, obs_interpretation, obs_status, obs_category, obs_subject, obs_effective, obs_nte_note, dr_result_links, dr_subject, dr_code, dr_status, dr_effective, dr_category.' }
                ],
                assemblyRulesDoc: [
                    { key: 'collapse_text_obx',   src: 'OBX.3 + OBX.2', fhir: 'Observation.valueString', desc: '<strong>Merge mode</strong> — controlled in the "Text Report Merging" card at the top of the Assembly tab. When enabled, consecutive TX/FT OBX segments sharing the same OBX.3 code are joined into one Observation.valueString (the HL7 continuation-segment pattern). Example: a 50-line surgical pathology report sent as OBX|1|TX … OBX|50|TX all with OBX.3=4050097^Surg Path produces one Observation instead of 50. Disable (or restrict to specific services via <code>collapse_text_obx_services</code>) when your discrete lab results also happen to use the same OBX.3 across rows. <em>Examples where merging is correct: AP (Anatomic Pathology), RAD (Radiology), SP (Surgical Path). Examples where merging should be OFF or restricted: CH (Chemistry), MB (Microbiology) — each analyte has a distinct OBX.3.</em>' },
                    { key: 'obs_value_dispatch',  src: 'OBX.2 + OBX.5', fhir: 'value[x]',              desc: 'Dispatches OBX value to the correct FHIR type: NM→valueQuantity (float), CE/CWE→valueCodeableConcept, ST/TX→valueString, TS→valueDateTime.' },
                    { key: 'obs_value_unit',       src: 'OBX.6',         fhir: 'valueQuantity.unit',     desc: 'Populates unit + system on valueQuantity. Normalizes UCUM coding system to http://unitsofmeasure.org.' },
                    { key: 'obs_code',             src: 'OBX.3',         fhir: 'Observation.code',       desc: 'Builds CodeableConcept from OBX.3. Normalizes LN → http://loinc.org, local codes are preserved with display text.' },
                    { key: 'obs_reference_range',  src: 'OBX.7',         fhir: 'referenceRange[]',       desc: 'Parses "3.8-11.0" into structured {low:{value,unit}, high:{value,unit}, text}. Disable if your system uses non-numeric or multi-part ranges.' },
                    { key: 'obs_interpretation',   src: 'OBX.8',         fhir: 'interpretation[]',       desc: 'Maps abnormal flags to FHIR interpretation codes: H→High, L→Low, A→Abnormal, AA→Critical, HH→Critical High, LL→Critical Low, N is ignored.' },
                    { key: 'obs_status',           src: 'OBX.11',        fhir: 'Observation.status',     desc: 'Maps OBX status code to FHIR: F→final, P→preliminary, C→corrected, W→entered-in-error, X→cancelled.' },
                    { key: 'obs_category',         src: '(fixed)',        fhir: 'Observation.category[]', desc: 'Adds the standard laboratory category coding (http://terminology.hl7.org/CodeSystem/observation-category | laboratory).' },
                    { key: 'obs_subject',          src: 'Patient.id',    fhir: 'Observation.subject',    desc: 'Links Observation.subject → Patient/<id> using the Patient resource already in the output.' },
                    { key: 'obs_effective',        src: 'OBX.14',        fhir: 'effectiveDateTime',      desc: 'Converts OBX.14 from HL7 TS format (20120410160227) to ISO 8601 (2012-04-10T16:02:27+00:00).' },
                    { key: 'obs_nte_note',         src: 'NTE.3',         fhir: 'note[].text',             desc: 'Clubs consecutive NTE segments following an OBX into Observation.note[0].text. Multiple NTE lines joined with newlines; blank lines become paragraph breaks. NTE after OBR (before first OBX) goes to DiagnosticReport.note.' },
                    { key: 'dr_result_links',      src: 'Observation ids', fhir: 'DiagnosticReport.result[]', desc: 'Populates DiagnosticReport.result[] with references to all assembled Observations.' },
                    { key: 'dr_subject',           src: 'Patient.id',    fhir: 'DiagnosticReport.subject', desc: 'Links DiagnosticReport.subject → Patient/<id>.' },
                    { key: 'dr_code',              src: 'OBR.4',         fhir: 'DiagnosticReport.code',  desc: 'Builds CodeableConcept from OBR.4 (ordered test / panel code). Required field in FHIR R4 DiagnosticReport.' },
                    { key: 'dr_status',            src: 'OBR.25',        fhir: 'DiagnosticReport.status', desc: 'Maps OBR result status to FHIR: F→final, P→preliminary, C→corrected, X→cancelled.' },
                    { key: 'dr_effective',         src: 'OBR.7',         fhir: 'effectiveDateTime',      desc: 'Converts OBR.7 observation date/time from HL7 TS to ISO 8601. Preferred over OBX.14 for the report-level timestamp.' },
                    { key: 'dr_category',          src: '(fixed)',        fhir: 'DiagnosticReport.category[]', desc: 'Adds the standard LAB category coding (http://terminology.hl7.org/CodeSystem/v2-0074 | LAB).' }
                ],
                referenceVariables: {
                    title: 'Using Enriched Data in Field Mappings',
                    description: 'Reference data from any upstream step using the ["step_name"].enriched_data.field syntax in the HL7 Source column.',
                    examples: [
                        {
                            scenario: 'Database lookup result',
                            hl7Field: '["database_enrichment"].enriched_data.insuranceProvider',
                            fhirPath: 'Coverage.payor[0].display',
                            explanation: 'Maps the insuranceProvider field fetched from a database enrichment step into Coverage.payor'
                        },
                        {
                            scenario: 'Script calculation',
                            hl7Field: '["Script_Enrichment"].enriched_data.riskScore',
                            fhirPath: 'Observation.valueQuantity.value',
                            explanation: 'Uses a risk score calculated by a prior Script Enrichment step as the Observation value'
                        },
                        {
                            scenario: 'API response field',
                            hl7Field: '["API_Enrichment"].enriched_data.externalPatientId',
                            fhirPath: 'Patient.identifier[1].value',
                            explanation: 'Adds an external system ID retrieved from an API call as a second Patient identifier'
                        }
                    ]
                }
            },
            'post.validation': {
                description: 'Validates FHIR data against the R4 specification. Works in two modes: <strong>Bundle mode</strong> (validates bundle structure + all entry resources) when <code>fhirBundle</code> contains a Bundle, and <strong>Resource mode</strong> (validates a single standalone resource) when <code>fhirResource</code> is present or <code>fhirBundle</code> contains a non-Bundle resource. Searches <code>fhirBundle</code>, <code>fhirResource</code>, <code>message.*</code>, and <code>enriched.*</code>.',
                useCases: [
                    'Validate FHIR bundle from HL7→FHIR Transform (bundle mode)',
                    'Validate a standalone FHIR resource from an API enrichment step (resource mode)',
                    'Validate required fields per resource type (14 types with hardcoded rules)',
                    'Check internal bundle references point to existing resources (bundle mode)',
                    'Enforce specific resource types must be present (e.g., Patient + Encounter)',
                    'Full R4 schema validation using 146 JSON schema definitions (strict mode)',
                    'Stop the pipeline on validation failure (fail_on_error: true)'
                ],
                example: {
                    validation_level: 'standard',
                    required_resources: ['Patient', 'Encounter'],
                    validate_references: true,
                    validate_required_fields: true,
                    fail_on_error: false
                },
                parameters: [
                    { name: 'validation_level', type: 'string', required: true, description: '"basic" (resourceType + id only), "standard" (+ required fields + references), "strict" (+ full R4 JSON schema). Default: "standard".' },
                    { name: 'required_resources', type: 'string[]', required: false, description: 'FHIR resource types that must be present. In bundle mode, checks all entries. In resource mode, checks the single resource matches. Example: ["Patient", "Encounter"].' },
                    { name: 'validate_references', type: 'boolean', required: false, description: 'Check that reference fields resolve to existing resources (by fullUrl or ResourceType/id). Most useful in bundle mode where references can be cross-checked. Default: true.' },
                    { name: 'validate_required_fields', type: 'boolean', required: false, description: 'Check FHIR-spec-required fields per resource type (e.g., Encounter must have status and class, Observation must have status and code). Default: true.' },
                    { name: 'fail_on_error', type: 'boolean', required: false, description: 'When true, the pipeline stops if any validation error is found. When false, errors are reported in step output but the pipeline continues. Default: false.' }
                ],
                validationTypes: [
                    {
                        type: 'basic',
                        description: 'Minimal structural check. Only verifies that each resource in the bundle has a resourceType and id.',
                        usedFor: 'Quick sanity check, development/debugging, high-throughput pipelines where speed matters.',
                        example: { validation_level: 'basic' }
                    },
                    {
                        type: 'standard',
                        description: 'Checks required fields per resource type using hardcoded rules for 14 common FHIR resource types. Also validates that internal bundle references point to existing resources.',
                        usedFor: 'Production pipelines. Catches common mapping errors like missing Encounter.status or MessageHeader.event.',
                        example: {
                            validation_level: 'standard',
                            required_resources: ['Patient', 'Encounter'],
                            validate_references: true,
                            validate_required_fields: true,
                            fail_on_error: true
                        }
                    },
                    {
                        type: 'strict',
                        description: 'Full R4 JSON schema validation using 146 FHIR resource schema definitions. Validates required fields from the official spec, element data types, and cardinality constraints.',
                        usedFor: 'Compliance validation, regulatory submissions, interoperability testing.',
                        example: {
                            validation_level: 'strict',
                            required_resources: ['Patient', 'Encounter', 'Observation'],
                            fail_on_error: true
                        }
                    }
                ]
            },
            'post.delivery': {
                description: 'Delivers transformed FHIR resources to target FHIR server or other destinations. Handles retries and error recovery.',
                useCases: [
                    'Send FHIR Bundle to healthcare FHIR server',
                    'Post individual resources to RESTful FHIR API',
                    'Deliver to multiple endpoints (primary + backup)',
                    'Archive to object storage (S3, Azure Blob)',
                    'Queue for async processing'
                ],
                example: {
                    endpoint: 'http://fhir-server:8080/fhir',
                    resource: 'Bundle',
                    method: 'POST',
                    retry_count: 3,
                    retry_delay_ms: 1000,
                    auth: {
                        type: 'bearer',
                        token: '${FHIR_TOKEN}'
                    }
                },
                parameters: [
                    { name: 'endpoint', type: 'string', required: true, description: 'FHIR server URL' },
                    { name: 'resource', type: 'string', required: true, description: 'Resource type to send (Patient, Bundle, etc.)' },
                    { name: 'retry_count', type: 'number', required: false, description: 'Number of retry attempts (default: 3)' }
                ]
            },
            'pre.logic.switch': {
                description: 'Routes messages through different processing paths based on field value matching. Evaluates a single field against multiple case values and executes corresponding actions. Supports multi-step routing where a single case can trigger a sequence of steps to execute in order. Ideal for message type routing, status-based processing, and complex multi-branch conditional workflows.',
                useCases: [
                    'Route messages by type (ADT^A01 → admission flow, ADT^A03 → discharge flow, ORU^R01 → lab flow)',
                    'Process by patient class (I → inpatient enrichment, O → outpatient enrichment, E → emergency fast-track)',
                    'Handle status codes (A → active processing, C → cancelled cleanup, P → pending queue)',
                    'Route by facility (FAC001 → Epic integration, FAC002 → Cerner integration, FAC003 → custom flow)',
                    'Process by priority (STAT → immediate, ROUTINE → normal queue, ASAP → expedited)',
                    'Branch by insurance type (MEDICARE → CMS validation, MEDICAID → state rules, COMMERCIAL → standard)',
                    'Multi-step workflows (ADT^A01 → [Validate Patient, Enrich Demographics, Route to ADT Handler])',
                    'Skip specific steps for certain cases (ORU^R01 → skip patient enrichment, go directly to lab processing)'
                ],
                example: {
                    description: 'Route by message type with multi-step execution',
                    field: 'MSH.9.1',
                    cases: [
                        {
                            value: 'ADT',
                            label: 'ADT Messages',
                            actions: [
                                { action: 'set_value', targetField: 'metadata.category', value: 'admission' },
                                { action: 'route_to_step', targetStepIds: ['validate-patient', 'enrich-demographics', 'adt-handler'] }
                            ]
                        },
                        {
                            value: 'ORU',
                            label: 'Lab Results',
                            actions: [
                                { action: 'set_value', targetField: 'metadata.category', value: 'lab' },
                                { action: 'route_to_step', targetStepIds: ['validate-results', 'lab-handler'] }
                            ]
                        },
                        {
                            value: 'ORM',
                            label: 'Orders',
                            actions: [
                                { action: 'route_to_step', targetStepId: 'order-handler' }
                            ]
                        }
                    ],
                    default: {
                        actions: [
                            { action: 'set_value', targetField: 'metadata.category', value: 'unknown' },
                            { action: 'route_to_step', targetStepIds: ['log-warning', 'error-handler'] }
                        ]
                    },
                    options: { caseInsensitive: false, trimWhitespace: true }
                },
                parameters: [
                    { name: 'field', type: 'string', required: true, description: 'HL7 field path to evaluate. Use the field selector to choose from available fields. Examples: "MSH.9.1" for message type code, "PV1.2" for patient class, "PID.8" for gender.' },
                    { name: 'cases', type: 'Array<CaseDefinition>', required: true, description: 'Array of case definitions. Each case has a value to match and actions to execute when matched. Cases are evaluated in order; first match wins.' },
                    { name: 'cases[].value', type: 'string', required: true, description: 'The value to match against the field. Exact match by default (use options.caseInsensitive for case-insensitive matching).' },
                    { name: 'cases[].label', type: 'string', required: false, description: 'Human-readable label for this case. Displayed in the UI for documentation purposes.' },
                    { name: 'cases[].actions', type: 'Array<Action>', required: true, description: 'Actions to execute when this case matches. Multiple actions can be defined and execute in order.' },
                    { name: 'default', type: 'object', required: false, description: 'Fallback configuration when no cases match. Contains actions array.' },
                    { name: 'default.actions', type: 'Array<Action>', required: false, description: 'Actions to execute when no cases match. Typically "continue" or error handling.' },
                    { name: 'options.caseInsensitive', type: 'boolean', required: false, description: 'Perform case-insensitive value matching. Default: false. Set to true for values like "M"/"m" or "STAT"/"stat".' },
                    { name: 'options.trimWhitespace', type: 'boolean', required: false, description: 'Trim leading/trailing whitespace before comparison. Default: true. Handles HL7 padding automatically.' }
                ],
                actions: [
                    {
                        action: 'continue',
                        description: 'Continue to the next step in sequence',
                        usedFor: 'Normal flow - process continues to the next step based on sequence number',
                        parameters: 'None'
                    },
                    {
                        action: 'stop',
                        description: 'Stop pipeline execution immediately',
                        usedFor: 'Halting processing for invalid/unsupported message types, or when no further processing needed',
                        parameters: 'None'
                    },
                    {
                        action: 'set_value',
                        description: 'Set a field value in the message data',
                        usedFor: 'Tagging messages with category, routing metadata, or computed values',
                        parameters: 'targetField (where to set), value (what to set)'
                    },
                    {
                        action: 'copy_field',
                        description: 'Copy value from one field to another',
                        usedFor: 'Duplicating field values, creating backup copies, or preparing data for downstream steps',
                        parameters: 'sourceField (copy from), targetField (copy to)'
                    },
                    {
                        action: 'transform',
                        description: 'Apply a transformation to a field value',
                        usedFor: 'Formatting field values - uppercase, lowercase, trim, capitalize, substring',
                        parameters: 'targetField (field to transform), transformType (uppercase|lowercase|trim|capitalize)'
                    },
                    {
                        action: 'route_to_step',
                        description: 'Route to one or more specific steps by ID',
                        usedFor: 'Branching to different processing paths. Supports multi-step routing for complex workflows.',
                        parameters: 'targetStepId (single step) OR targetStepIds (array of steps to execute in order)'
                    },
                    {
                        action: 'skip_steps',
                        description: 'Skip specified steps and continue',
                        usedFor: 'Bypassing steps that are not relevant for this case (e.g., skip patient enrichment for lab messages)',
                        parameters: 'skipStepIds (array of step IDs to skip)'
                    }
                ],
                multiStepRouting: {
                    title: 'Multi-Step Routing',
                    description: 'A single case can route to multiple steps that execute in sequence. This enables complex workflows where one condition triggers a chain of processing steps.',
                    howToUse: [
                        'In the Configuration tab, select a case',
                        'Choose "Route to Step(s)" action',
                        'Use the dropdown to add multiple target steps',
                        'Steps are displayed as numbered chips (1. Step A, 2. Step B, etc.)',
                        'Click × on any chip to remove a step from the sequence'
                    ],
                    example: {
                        scenario: 'ADT messages need validation, enrichment, then specialized handling',
                        config: {
                            value: 'ADT',
                            actions: [{
                                action: 'route_to_step',
                                targetStepIds: ['validate-patient', 'enrich-demographics', 'adt-handler']
                            }]
                        },
                        execution: 'validate-patient runs first, then enrich-demographics, then adt-handler'
                    }
                },
                comparisonWithIfThenElse: {
                    title: 'Switch/Case vs If-Then-Else',
                    description: 'Both steps support conditional logic but are optimized for different scenarios:',
                    comparison: [
                        { feature: 'Best for', switchCase: 'Single field with multiple possible values', ifThenElse: 'Complex conditions with multiple fields and operators' },
                        { feature: 'Condition type', switchCase: 'Exact value matching (equals)', ifThenElse: 'Any comparison (equals, contains, greater than, less than, regex, is_empty)' },
                        { feature: 'Number of branches', switchCase: 'Many branches (3+ cases)', ifThenElse: 'Two branches (true path / false path)' },
                        { feature: 'Default handling', switchCase: 'Explicit default case', ifThenElse: 'Else branch for false conditions' },
                        { feature: 'Use Case Example', switchCase: 'Route ADT^A01/A02/A03/A04 to different flows', ifThenElse: 'If patient age > 65 AND has chronic condition, flag high-risk' }
                    ],
                    recommendation: 'Use Switch/Case when you have a single field with many possible values (like message type, patient class, facility code). Use If-Then-Else when you need complex boolean logic combining multiple conditions.'
                },
                bestPractices: [
                    {
                        practice: 'Use meaningful case labels',
                        reason: 'Labels make the configuration self-documenting and easier to maintain',
                        example: 'Label "Admission" for value "ADT^A01" instead of leaving blank'
                    },
                    {
                        practice: 'Always define a default case',
                        reason: 'Handles unexpected values gracefully instead of silent failures',
                        example: 'Default: log warning + continue, or route to error handler'
                    },
                    {
                        practice: 'Use caseInsensitive for user-entered data',
                        reason: 'HL7 data may have inconsistent casing (e.g., "M" vs "m" for male)',
                        example: 'Enable caseInsensitive for PID.8 (gender), PV1.2 (patient class)'
                    },
                    {
                        practice: 'Keep actions simple per case',
                        reason: 'Complex logic should be in dedicated steps, not crammed into switch actions',
                        example: 'Use route_to_step to branch to specialized processing steps rather than multiple set_value actions'
                    },
                    {
                        practice: 'Test with edge cases',
                        reason: 'Empty values, null fields, and whitespace can cause unexpected matching',
                        example: 'Test with empty MSH.9, whitespace-padded values, and unexpected message types'
                    }
                ],
                troubleshooting: [
                    {
                        issue: 'Case not matching expected value',
                        cause: 'Whitespace in field value, case sensitivity mismatch, or wrong field path',
                        fix: 'Enable trimWhitespace option (default), enable caseInsensitive option if needed, and verify field path using field selector'
                    },
                    {
                        issue: 'Default case always executing',
                        cause: 'Field path returns null/undefined, no cases defined, or field value has unexpected format',
                        fix: 'Check field path is correct using field selector, add case definitions, and use test message to verify actual field values'
                    },
                    {
                        issue: 'Multi-step routing not executing all steps',
                        cause: 'Step IDs are incorrect, target steps are disabled, or an earlier step has a stop action',
                        fix: 'Verify step IDs match exactly (case-sensitive), check that target steps are enabled, and review step configurations for stop actions'
                    },
                    {
                        issue: 'Actions not being applied',
                        cause: 'Wrong action type selected, missing required parameters, or target field path invalid',
                        fix: 'Verify action type matches your intent, ensure all required parameters are filled (e.g., targetField for set_value), and check field paths are valid'
                    }
                ]
            },
            'control.loop': {
                description: 'Container step that executes nested steps in a loop. Supports three loop types: For Each (iterate over collections like OBX segments), For (repeat N times), and While (condition-based). Child steps are visually nested inside the loop container and execute on each iteration with access to loop variables.',
                useCases: [
                    'Process all OBX segments in a lab result message (For Each over enhancedSegments.OBX)',
                    'Transform multiple diagnosis codes (For Each over DG1 segments)',
                    'Retry failed operations up to N times (For loop with retry logic)',
                    'Poll external API until data is ready (While loop with condition check)',
                    'Process patient encounters in batch (For Each over encounter list)',
                    'Apply transformations to all observations (For Each with nested transform steps)',
                    'Generate multiple FHIR resources from repeating HL7 segments'
                ],
                example: {
                    description: 'Process all OBX segments and transform to FHIR Observations',
                    loopType: 'foreach',
                    collection: 'enhancedSegments.OBX',
                    itemVariable: 'observation',
                    indexVariable: 'index',
                    childStepIds: ['validate-obx', 'transform-to-fhir', 'enrich-observation'],
                    maxIterations: 1000,
                    breakOnError: false,
                    continueOnEmpty: true
                },
                parameters: [
                    { name: 'loopType', type: 'enum', required: true, description: 'Type of loop: "foreach" (iterate collection), "for" (repeat N times), "while" (condition-based)' },
                    { name: 'collection', type: 'string', required: false, description: 'For "foreach": Field path to the array/collection to iterate over. Example: "enhancedSegments.OBX" for all OBX segments.' },
                    { name: 'itemVariable', type: 'string', required: false, description: 'Variable name for current item. Access as loop.{name} in child steps. Default: "item"' },
                    { name: 'indexVariable', type: 'string', required: false, description: 'Variable name for current index. Access as loop.{name} in child steps. Default: "index"' },
                    { name: 'iterations', type: 'number', required: false, description: 'For "for" loops: Number of times to execute the loop body.' },
                    { name: 'condition', type: 'object', required: false, description: 'For "while" loops: Condition object with field, operator, and value. Loop continues while condition is true.' },
                    { name: 'childStepIds', type: 'Array<string>', required: true, description: 'IDs of steps to execute in the loop body. Steps execute in order for each iteration.' },
                    { name: 'maxIterations', type: 'number', required: false, description: 'Safety limit to prevent infinite loops. Default: 1000' },
                    { name: 'breakOnError', type: 'boolean', required: false, description: 'Stop loop execution if a child step fails. Default: false (continue to next iteration)' },
                    { name: 'continueOnEmpty', type: 'boolean', required: false, description: 'Continue pipeline if collection is empty. Default: true' }
                ],
                loopVariables: {
                    title: 'Available Loop Variables',
                    description: 'Variables available inside child steps during loop execution:',
                    variables: [
                        { name: 'loop.{itemVariable}', description: 'Current item being processed (For Each only)', example: 'loop.observation' },
                        { name: 'loop.{indexVariable}', description: 'Current iteration index (0-based)', example: 'loop.index' },
                        { name: 'loop.iteration', description: 'Current iteration number (1-based)', example: 'loop.iteration' },
                        { name: 'loop.isFirst', description: 'True if this is the first iteration', example: 'loop.isFirst' },
                        { name: 'loop.isLast', description: 'True if this is the last iteration', example: 'loop.isLast' },
                        { name: 'loop.length', description: 'Total number of items (For Each only)', example: 'loop.length' },
                        { name: 'loop.total', description: 'Total number of iterations (For loop only)', example: 'loop.total' }
                    ]
                },
                loopTypes: {
                    title: 'Loop Type Comparison',
                    types: [
                        {
                            type: 'foreach',
                            name: 'For Each',
                            description: 'Iterates over each item in a collection',
                            useCase: 'Process all OBX segments, transform multiple diagnosis codes',
                            config: 'Requires: collection path'
                        },
                        {
                            type: 'for',
                            name: 'For (N times)',
                            description: 'Repeats execution a fixed number of times',
                            useCase: 'Retry operations, batch processing with fixed count',
                            config: 'Requires: iterations count'
                        },
                        {
                            type: 'while',
                            name: 'While',
                            description: 'Continues while a condition is true',
                            useCase: 'Poll until ready, process until quota reached',
                            config: 'Requires: condition (field, operator, value)'
                        }
                    ]
                },
                bestPractices: [
                    {
                        practice: 'Always set maxIterations',
                        reason: 'Prevents infinite loops that could hang the pipeline',
                        example: 'Set maxIterations to a reasonable value like 100 or 1000 based on expected data size'
                    },
                    {
                        practice: 'Use meaningful variable names',
                        reason: 'Makes child step configuration more readable',
                        example: 'Use "observation" instead of "item" when iterating OBX segments'
                    },
                    {
                        practice: 'Consider breakOnError setting',
                        reason: 'Decide if one failed iteration should stop all processing',
                        example: 'Set breakOnError=true for critical data, false for best-effort processing'
                    },
                    {
                        practice: 'Handle empty collections',
                        reason: 'Messages may not always have the expected segments',
                        example: 'Set continueOnEmpty=true to gracefully handle messages without OBX segments'
                    }
                ],
                troubleshooting: [
                    {
                        issue: 'Loop not executing any iterations',
                        cause: 'Collection path is incorrect or collection is empty',
                        fix: 'Verify collection path using the field selector. Check if continueOnEmpty is set correctly.'
                    },
                    {
                        issue: 'Loop stops after max iterations',
                        cause: 'maxIterations limit reached',
                        fix: 'Increase maxIterations if you expect more items, or check for infinite loop conditions in While loops'
                    },
                    {
                        issue: 'Child steps not receiving loop variables',
                        cause: 'Variable names not matching between loop config and child step config',
                        fix: 'Ensure itemVariable and indexVariable names match what child steps expect'
                    },
                    {
                        issue: 'While loop runs forever',
                        cause: 'Condition never becomes false',
                        fix: 'Verify condition field is being updated by child steps. Always have maxIterations as safety limit.'
                    }
                ]
            },
        };

        // Check for general documentation requests (not step-specific)
        if (stepType === '_general.step_output_chaining') {
            return {
                description: 'Use output from previous steps in subsequent steps. Every step can store its output data in the pipeline execution context, and later steps can access this data to create powerful data transformation workflows.',
                useCases: [
                    'Multi-Database Enrichment - Use patient insurance ID from MySQL step to query PostgreSQL insurance details table',
                    'API-Database Hybrid - Verify patient exists in EHR via API, then only if verified, fetch detailed records from database',
                    'Validation-Enrichment Chain - Check required fields exist, then only if valid, enrich with external data',
                    'Conditional Processing - Execute steps based on results from previous steps',
                    'Data Aggregation - Combine results from multiple enrichment steps into single output'
                ],
                example: {
                    step1: {
                        stepName: 'MySQL Patient Lookup',
                        stepAlias: 'patient_lookup',
                        sequence: 10,
                        query: 'SELECT insurance_id FROM patients WHERE mrn = ?',
                        output: {
                            enriched_data: [{ insurance_id: 'INS123456' }],
                            rows_count: 1
                        }
                    },
                    step2: {
                        stepName: 'PostgreSQL Insurance Details',
                        stepAlias: 'insurance_details',
                        sequence: 20,
                        query: 'SELECT policy_number, coverage FROM insurance WHERE id = $1',
                        queryParams: {
                            '1': 'stepOutput.patient_lookup.enriched_data[0].insurance_id'
                        },
                        note: 'Uses insurance_id from step 1!'
                    }
                },
                accessMethods: [
                    {
                        method: 'By Step Alias (Recommended)',
                        format: 'stepOutput.<alias>.<field>',
                        examples: [
                            'stepOutput.patient_lookup.enriched_data',
                            'stepOutput.api_verification.response.patient_name',
                            'stepOutput.validation.is_valid'
                        ],
                        description: 'Use the user-friendly alias you assigned to the step'
                    },
                    {
                        method: 'Array Access',
                        format: 'stepOutput.<alias>.enriched_data[0].<column>',
                        examples: [
                            'stepOutput.patient_lookup.enriched_data[0].insurance_id',
                            'stepOutput.database_query.enriched_data[0].provider_npi'
                        ],
                        description: 'Access specific row and column from database results'
                    },
                    {
                        method: 'Nested Fields',
                        format: 'stepOutput.<alias>.response.nested.field',
                        examples: [
                            'stepOutput.api_call.response.patient.insurance.provider',
                            'stepOutput.empi_lookup.response.demographics.address.zip'
                        ],
                        description: 'Access deeply nested fields in API responses or JSON data'
                    }
                ],
                stepOutputStructure: {
                    database_enrichment: {
                        stepID: 'uuid-123',
                        stepName: 'Database Enrichment MySQL',
                        stepAlias: 'patient_lookup',
                        stepType: 'database_enrichment',
                        sequence: 10,
                        outputData: {
                            enriched_data: [
                                { insurance_id: 'INS123', patient_name: 'John Doe', mrn: 'P123456' }
                            ],
                            rows_count: 1,
                            query_params: { '1': 'P123456' }
                        },
                        success: true
                    },
                    api_enrichment: {
                        stepID: 'uuid-456',
                        stepName: 'API Enrichment Insurance',
                        stepAlias: 'insurance_verification',
                        stepType: 'api_enrichment',
                        sequence: 20,
                        outputData: {
                            response: {
                                policy_number: 'POL789',
                                coverage: 'Active',
                                provider: 'Blue Cross'
                            },
                            status_code: 200,
                            content_type: 'application/json'
                        },
                        success: true
                    }
                },
                workflow: [
                    { step: 1, action: 'Assign Step Alias', description: 'Give each step a meaningful alias (e.g., "patient_lookup", "insurance_check")' },
                    { step: 2, action: 'Configure First Step', description: 'Set up your first enrichment step normally (database query, API call, etc.)' },
                    { step: 3, action: 'Reference Previous Step', description: 'In subsequent steps, use stepOutput.<alias>.<field> in query parameters or field mappings' },
                    { step: 4, action: 'Use Field Search', description: 'The Field Path Search component shows available step outputs from previous steps' },
                    { step: 5, action: 'Test Pipeline', description: 'Run test to see step outputs flow through the pipeline' }
                ],
                bestPractices: [
                    {
                        practice: 'Use meaningful step aliases',
                        reason: 'Makes step output references self-documenting and easier to understand',
                        example: 'Use "empi_lookup" instead of "database_step_1"'
                    },
                    {
                        practice: 'Check array length before accessing',
                        reason: 'Database queries might return 0 rows, causing undefined errors',
                        example: 'Use conditional logic: if stepOutput.query1.rows_count > 0, then access [0]'
                    },
                    {
                        practice: 'Use different target paths for each step',
                        reason: 'Prevents output from one step overwriting another',
                        example: 'Step 1: enriched.empi, Step 2: enriched.insurance, Step 3: enriched.provider'
                    },
                    {
                        practice: 'Order steps by sequence number',
                        reason: 'Steps execute in sequence order - ensure dependencies come first',
                        example: 'Patient lookup (seq 10) → Insurance lookup (seq 20) → Provider lookup (seq 30)'
                    }
                ],
                troubleshooting: [
                    {
                        issue: 'stepOutput.<alias> is undefined',
                        cause: 'Referenced step has not executed yet or failed',
                        fix: 'Check sequence order - dependency must have lower sequence number. Check if previous step succeeded.'
                    },
                    {
                        issue: 'Cannot read property of undefined',
                        cause: 'Accessing nested field that doesn\'t exist in step output',
                        fix: 'Check step output structure in logs. Verify field path is correct.'
                    },
                    {
                        issue: 'Array index out of bounds',
                        cause: 'Database query returned 0 rows but code tries to access [0]',
                        fix: 'Check rows_count first: stepOutput.query.rows_count before accessing stepOutput.query.enriched_data[0]'
                    }
                ],
                parameters: [
                    {
                        name: 'stepOutput',
                        type: 'object',
                        required: false,
                        description: 'Global object containing outputs from all previous steps, keyed by step alias'
                    },
                    {
                        name: 'stepOutput.<alias>',
                        type: 'object',
                        required: false,
                        description: 'Output from specific step. Contains: enriched_data (for database), response (for API), validation_errors (for validation), etc.'
                    },
                    {
                        name: 'stepOutput.<alias>.enriched_data',
                        type: 'array',
                        required: false,
                        description: 'Array of database query results. Use [0] to access first row, [1] for second row, etc.'
                    },
                    {
                        name: 'stepOutput.<alias>.response',
                        type: 'object',
                        required: false,
                        description: 'API response data. Structure depends on API endpoint.'
                    },
                    {
                        name: 'stepOutput.<alias>.rows_count',
                        type: 'number',
                        required: false,
                        description: 'Number of rows returned by database query. Use to check if results exist before accessing [0]'
                    }
                ]
            };
        }

        // Connector step documentation
        docs['connector.inbound'] = {
            description: 'Starts a long-lived listener that receives messages from external systems. The most common type is TCP/MLLP Inbound which receives HL7 v2 messages from EHRs, lab systems, and other healthcare senders. Placed at sequence 5 in a pipeline — the engine starts one listener goroutine per connector.inbound step.',
            useCases: [
                'Receive HL7 ADT, ORU, or SIU messages from an EHR via TCP/MLLP (port 2575)',
                'Listen on multiple ports simultaneously by adding multiple connector.inbound steps',
                'Accept TLS-encrypted MLLP connections from external partners',
                'Receive messages and send AA/AE/AR acknowledgments back to the sender'
            ],
            example: {
                connectorType: 'tcp_mllp',
                config: {
                    host: '0.0.0.0',
                    port: 2575,
                    max_connections: 10,
                    ack: {
                        mode: 'immediate',
                        on_error: 'suppress',
                        sending_app: 'ezHealthKonnect',
                        sending_facility: 'EHK',
                        text_success: 'Message received successfully',
                        text_error: 'Message processing error'
                    }
                }
            },
            examples: [
                {
                    label: 'Basic TCP/MLLP — receive HL7 on port 6610 with immediate ACK',
                    config: {
                        connectorType: 'tcp_mllp',
                        config: {
                            host: '0.0.0.0',
                            port: 6610,
                            max_connections: 20,
                            timeout_seconds: 300,
                            ack: {
                                mode: 'immediate',
                                on_error: 'suppress',
                                sending_app: 'ezHealthKonnect',
                                sending_facility: 'EHK',
                                text_success: 'Message received',
                                text_error: 'Processing error'
                            }
                        }
                    }
                },
                {
                    label: 'TCP/MLLP with TLS — encrypted connections (port 2576)',
                    config: {
                        connectorType: 'tcp_mllp',
                        config: {
                            host: '0.0.0.0',
                            port: 2576,
                            enable_tls: true,
                            tls_cert_path: '/certs/server.crt',
                            tls_key_path: '/certs/server.key',
                            max_connections: 10,
                            ack: {
                                mode: 'immediate',
                                on_error: 'nack',
                                sending_app: 'SecureHIS',
                                sending_facility: 'PROD'
                            }
                        }
                    }
                },
                {
                    label: 'Custom ACK script — reject non-ADT messages with AR, accept all others with AA',
                    description: 'Use a custom script when you need conditional ACK logic beyond the standard mode/on_error settings. The function must be named buildACK(msg) and return { ackCode, textMessage }. Valid codes: AA (Accept), AE (Application Error), AR (Application Reject).',
                    config: {
                        connectorType: 'tcp_mllp',
                        config: {
                            host: '0.0.0.0',
                            port: 2575,
                            ack: {
                                mode: 'immediate',
                                on_error: 'nack',
                                sending_app: 'ezHealthKonnect',
                                sending_facility: 'EHK',
                                script: [
                                    'function buildACK(msg) {',
                                    '  // msg properties available:',
                                    '  //   msg.controlID      — MSH-10 message control ID',
                                    '  //   msg.messageType    — e.g. "ADT^A01", "ORU^R01"',
                                    '  //   msg.sendingApp     — MSH-3',
                                    '  //   msg.sendingFacility — MSH-4',
                                    '  //   msg.raw            — full raw HL7 message string',
                                    '  //   msg.defaultCode    — "AA" or "AE" (from mode/on_error)',
                                    '  //   msg.defaultText    — default MSA-3 text',
                                    '  var type = (msg.messageType || "").split("^")[0];',
                                    '  if (type !== "ADT" && type !== "ORU") {',
                                    '    return {',
                                    '      ackCode: "AR",',
                                    '      textMessage: "Unsupported message type: " + type',
                                    '    };',
                                    '  }',
                                    '  return {',
                                    '    ackCode: "AA",',
                                    '    textMessage: "Message accepted"',
                                    '  };',
                                    '}'
                                ].join('\n')
                            }
                        }
                    }
                },
                {
                    label: 'Custom ACK script — add patient ID to ACK text from parsed HL7',
                    config: {
                        connectorType: 'tcp_mllp',
                        config: {
                            host: '0.0.0.0',
                            port: 2575,
                            ack: {
                                mode: 'immediate',
                                on_error: 'suppress',
                                sending_app: 'ezHealthKonnect',
                                sending_facility: 'EHK',
                                script: [
                                    'function buildACK(msg) {',
                                    '  // Extract PID-3 (patient ID) from raw HL7',
                                    '  var patientId = "";',
                                    '  var lines = (msg.raw || "").split("\\r");',
                                    '  for (var i = 0; i < lines.length; i++) {',
                                    '    if (lines[i].indexOf("PID") === 0) {',
                                    '      var fields = lines[i].split("|");',
                                    '      patientId = fields[3] || "";',
                                    '      break;',
                                    '    }',
                                    '  }',
                                    '  return {',
                                    '    ackCode: "AA",',
                                    '    textMessage: patientId',
                                    '      ? "Accepted patient " + patientId',
                                    '      : "Message received"',
                                    '  };',
                                    '}'
                                ].join('\n')
                            }
                        }
                    }
                }
            ],
            connectorTypeCards: [
                {
                    typeName: 'tcp_mllp', displayName: 'TCP/MLLP (HL7 v2.x)', icon: '🔌', mode: 'push',
                    description: 'Long-lived TCP socket listener using the MLLP (Minimal Lower Layer Protocol) framing. The primary transport for HL7 v2 messages between EHRs, lab systems, and healthcare intermediaries. Sends AA/AE/AR acknowledgments back to the sender.',
                    notes: 'Supports TLS 1.2/1.3. Configure ACK behaviour in the Acknowledgment tab.',
                    required: ['port'],
                    keyFields: [
                        { name: 'port', type: 'integer', required: true, default: '2575', notes: 'Standard MLLP port. Use 2576 for TLS.' },
                        { name: 'host', type: 'string', default: '0.0.0.0', notes: 'Bind address. 0.0.0.0 listens on all interfaces.' },
                        { name: 'max_connections', type: 'integer', default: '10', notes: 'Max simultaneous HL7 sender connections.' },
                        { name: 'enable_tls', type: 'boolean', default: 'false', notes: 'Require TLS; also set tls_cert_path and tls_key_path.' },
                        { name: 'tls_cert_path', type: 'string', default: '—', notes: 'Path to PEM certificate file (when TLS enabled).' },
                        { name: 'tls_key_path', type: 'string', default: '—', notes: 'Path to PEM private key file (when TLS enabled).' }
                    ],
                    example: { connectorType: 'tcp_mllp', config: { host: '0.0.0.0', port: 2575, max_connections: 10, ack: { mode: 'immediate', on_error: 'suppress' } } }
                },
                {
                    typeName: 'http_rest', displayName: 'HTTP/REST API', icon: '🌐', mode: 'push',
                    description: 'Exposes an HTTP endpoint that external systems POST messages to. Useful for FHIR R4 sources, webhooks, and any system that speaks REST. Supports API key and bearer token authentication.',
                    required: ['endpoint_path'],
                    keyFields: [
                        { name: 'endpoint_path', type: 'string', required: true, default: '/api/hl7/receive', notes: 'URL path the server listens on.' },
                        { name: 'http_methods', type: 'array', default: '["POST"]', notes: 'Allowed HTTP methods: POST or PUT.' },
                        { name: 'authentication_type', type: 'enum', default: 'api_key', notes: 'none | api_key | basic_auth | bearer_token.' },
                        { name: 'api_key_header', type: 'string', default: 'X-API-Key', notes: 'Header name carrying the API key.' }
                    ],
                    example: { connectorType: 'http_rest', config: { endpoint_path: '/api/hl7/receive', http_methods: ['POST'], authentication_type: 'api_key', api_key_header: 'X-API-Key' } }
                },
                {
                    typeName: 'sftp_inbound', displayName: 'SFTP File Reader', icon: '🔐', mode: 'pull',
                    description: 'Polls a remote SFTP server directory for new files on a configurable schedule (cron). After downloading each file it can delete, move, or rename it on the server. Supports both password and SSH private key authentication.',
                    notes: 'Requires a cron schedule on the interface. Set "After Processing" to delete or move to prevent reprocessing the same file.',
                    required: ['host', 'port', 'username', 'remote_directory'],
                    keyFields: [
                        { name: 'host', type: 'string', required: true, default: '—', notes: 'SFTP server hostname or IP.' },
                        { name: 'port', type: 'integer', required: true, default: '22', notes: 'Standard SSH/SFTP port.' },
                        { name: 'username', type: 'string', required: true, default: '—', notes: 'SSH login username.' },
                        { name: 'password', type: 'string (password)', default: '—', notes: 'SSH password. Leave blank if using private key.' },
                        { name: 'private_key_path', type: 'string', default: '—', notes: 'Path to SSH private key file (alternative to password).' },
                        { name: 'remote_directory', type: 'string', required: true, default: '—', notes: 'Remote path to poll for new files.' },
                        { name: 'file_pattern', type: 'string', default: '*.hl7', notes: 'Glob pattern to match files (e.g. *.hl7, ADT_*.txt).' },
                        { name: 'after_processing', type: 'enum', default: 'move', notes: 'delete | move | rename | nothing.' },
                        { name: 'archive_directory', type: 'string', default: '—', notes: 'Remote path to move processed files to.' },
                        { name: 'max_files_per_poll', type: 'integer', default: '100', notes: 'Cap per cron run to avoid overload.' }
                    ],
                    example: { connectorType: 'sftp_inbound', config: { host: 'sftp.partner.org', port: 22, username: 'hl7feed', private_key_path: '/certs/sftp_key', remote_directory: '/outbound/hl7', file_pattern: '*.hl7', after_processing: 'move', archive_directory: '/processed/hl7', max_files_per_poll: 50 } }
                },
                {
                    typeName: 'file_listener', displayName: 'File System Listener', icon: '📁', mode: 'pull',
                    description: 'Monitors a local directory on the server/container filesystem for new files on a schedule. Best for shared NFS mounts, local drop folders, or integration test scenarios where files are placed by another process.',
                    notes: 'Requires a cron schedule. The directory must be accessible inside the container — use a Docker volume mount.',
                    required: ['directory_path', 'file_pattern'],
                    keyFields: [
                        { name: 'directory_path', type: 'string', required: true, default: '—', notes: 'Absolute path on server, e.g. /data/hl7/inbox.' },
                        { name: 'file_pattern', type: 'string', required: true, default: '*.hl7', notes: 'Glob to match new files.' },
                        { name: 'after_processing', type: 'enum', default: 'move', notes: 'delete | move | archive | nothing.' },
                        { name: 'archive_directory', type: 'string', default: '—', notes: 'Path to move processed files.' },
                        { name: 'file_encoding', type: 'string', default: 'UTF-8', notes: 'Character encoding of incoming files.' }
                    ],
                    example: { connectorType: 'file_listener', config: { directory_path: '/data/hl7/inbox', file_pattern: '*.hl7', after_processing: 'move', archive_directory: '/data/hl7/archive' } }
                },
                {
                    typeName: 'postgresql_inbound', displayName: 'PostgreSQL Database Reader', icon: '🐘', mode: 'pull',
                    description: 'Polls a PostgreSQL table or view for new records on a schedule. Use an incremental column (e.g. id or updated_at) to fetch only new/changed rows. After fetching, can mark rows as processed via an update flag or delete them.',
                    notes: 'Requires a cron schedule. Use incremental_column to avoid reprocessing rows on every poll.',
                    required: ['host', 'port', 'database', 'query'],
                    keyFields: [
                        { name: 'host', type: 'string', required: true, default: 'localhost', notes: 'PostgreSQL host.' },
                        { name: 'port', type: 'integer', required: true, default: '5432', notes: 'PostgreSQL port.' },
                        { name: 'database', type: 'string', required: true, default: '—', notes: 'Database name.' },
                        { name: 'username', type: 'string', default: '—', notes: 'Login user.' },
                        { name: 'password', type: 'string (password)', default: '—', notes: 'Login password.' },
                        { name: 'query', type: 'string', required: true, default: '—', notes: 'SELECT query to fetch records. Use WHERE processed_flag = false for incremental.' },
                        { name: 'incremental_column', type: 'string', default: '—', notes: 'Column to track last polled value (id or updated_at).' },
                        { name: 'after_processing', type: 'enum', default: 'update_flag', notes: 'update_flag | delete | archive | nothing.' },
                        { name: 'update_column', type: 'string', default: '—', notes: 'Column to set when after_processing=update_flag.' },
                        { name: 'update_value', type: 'string', default: 'processed', notes: 'Value to set in update_column.' },
                        { name: 'max_records_per_poll', type: 'integer', default: '100', notes: 'LIMIT applied to the query per cron run.' }
                    ],
                    example: { connectorType: 'postgresql_inbound', config: { host: 'db.internal', port: 5432, database: 'ehr', username: 'reader', password: '••••', query: 'SELECT * FROM hl7_outbox WHERE processed = false ORDER BY created_at LIMIT 100', after_processing: 'update_flag', update_column: 'processed', update_value: 'true', max_records_per_poll: 100 } }
                },
                {
                    typeName: 'rabbitmq_inbound', displayName: 'RabbitMQ Consumer', icon: '🐰', mode: 'push',
                    description: 'Connects to a RabbitMQ broker and consumes messages from a queue using AMQP. Long-lived subscription — no cron needed. Supports exchange binding, manual acknowledgment, and TLS connections.',
                    required: ['host', 'port', 'queue_name'],
                    keyFields: [
                        { name: 'host', type: 'string', required: true, default: 'localhost', notes: 'RabbitMQ broker host.' },
                        { name: 'port', type: 'integer', required: true, default: '5672', notes: 'AMQP port (5671 for TLS).' },
                        { name: 'username', type: 'string', default: 'guest', notes: 'AMQP username.' },
                        { name: 'password', type: 'string (password)', default: 'guest', notes: 'AMQP password.' },
                        { name: 'vhost', type: 'string', default: '/', notes: 'RabbitMQ virtual host.' },
                        { name: 'queue_name', type: 'string', required: true, default: '—', notes: 'Queue to consume from.' },
                        { name: 'exchange_name', type: 'string', default: '—', notes: 'Optional exchange to bind the queue to.' },
                        { name: 'routing_key', type: 'string', default: '—', notes: 'Routing key for exchange binding.' },
                        { name: 'prefetch_count', type: 'integer', default: '1', notes: 'Messages to prefetch per consumer (flow control).' },
                        { name: 'auto_ack', type: 'boolean', default: 'false', notes: 'false = manual ack (safer); true = auto-ack.' }
                    ],
                    example: { connectorType: 'rabbitmq_inbound', config: { host: 'rabbitmq.internal', port: 5672, username: 'hl7consumer', password: '••••', vhost: '/healthcare', queue_name: 'hl7.inbound.adt', prefetch_count: 5, auto_ack: false, durable_queue: true } }
                },
                {
                    typeName: 'kafka_inbound', displayName: 'Kafka Consumer', icon: '⚡', mode: 'push',
                    description: 'Subscribes to a Kafka topic as a consumer group member. Long-lived connection — no cron needed. Offsets are committed after successful processing. Supports SASL/SCRAM authentication and SSL for secured Kafka clusters.',
                    required: ['bootstrap_servers', 'topic', 'group_id'],
                    keyFields: [
                        { name: 'bootstrap_servers', type: 'string', required: true, default: 'localhost:9092', notes: 'Comma-separated broker list, e.g. broker1:9092,broker2:9092.' },
                        { name: 'topic', type: 'string', required: true, default: '—', notes: 'Kafka topic name to consume from.' },
                        { name: 'group_id', type: 'string', required: true, default: '—', notes: 'Consumer group ID — use a unique ID per pipeline.' },
                        { name: 'client_id', type: 'string', default: '—', notes: 'Optional client identifier for monitoring.' },
                        { name: 'auto_offset_reset', type: 'enum', default: 'latest', notes: 'earliest = read from start; latest = only new messages.' },
                        { name: 'security_protocol', type: 'enum', default: 'PLAINTEXT', notes: 'PLAINTEXT | SSL | SASL_PLAINTEXT | SASL_SSL.' },
                        { name: 'sasl_mechanism', type: 'enum', default: 'PLAIN', notes: 'PLAIN | SCRAM-SHA-256 | SCRAM-SHA-512.' },
                        { name: 'sasl_username', type: 'string', default: '—', notes: 'SASL username for secured clusters.' },
                        { name: 'sasl_password', type: 'string (password)', default: '—', notes: 'SASL password.' },
                        { name: 'max_poll_records', type: 'integer', default: '500', notes: 'Max records per poll cycle.' }
                    ],
                    example: { connectorType: 'kafka_inbound', config: { bootstrap_servers: 'kafka1:9092,kafka2:9092', topic: 'hl7.inbound.adt', group_id: 'ezHealthKonnect-adt-pipeline', auto_offset_reset: 'latest', security_protocol: 'SASL_SSL', sasl_mechanism: 'SCRAM-SHA-256', sasl_username: 'hl7consumer', sasl_password: '••••' } }
                },
                {
                    typeName: 'redis_inbound', displayName: 'Redis Consumer', icon: '🔴', mode: 'push',
                    description: 'Consumes messages from Redis using list (LPOP/BRPOP), pub/sub channel subscription, or Redis Streams. Long-lived connection — no cron needed. List mode with blocking timeout is the simplest queue pattern.',
                    required: ['host', 'port', 'mode'],
                    keyFields: [
                        { name: 'host', type: 'string', required: true, default: 'localhost', notes: 'Redis host.' },
                        { name: 'port', type: 'integer', required: true, default: '6379', notes: 'Redis port.' },
                        { name: 'password', type: 'string (password)', default: '—', notes: 'AUTH password (if Redis requires auth).' },
                        { name: 'database', type: 'integer', default: '0', notes: 'Redis DB number (0–15).' },
                        { name: 'mode', type: 'enum', required: true, default: 'list_pop', notes: 'list_pop | pub_sub | stream.' },
                        { name: 'key_name', type: 'string', default: '—', notes: 'List key / channel name / stream name.' },
                        { name: 'consumer_group', type: 'string', default: '—', notes: 'For stream mode — consumer group name.' },
                        { name: 'blocking_timeout', type: 'integer', default: '0', notes: 'BLPOP timeout in seconds; 0 = block indefinitely.' }
                    ],
                    example: { connectorType: 'redis_inbound', config: { host: 'redis.internal', port: 6379, password: '••••', database: 0, mode: 'list_pop', key_name: 'hl7:queue:adt', blocking_timeout: 30 } }
                },
                {
                    typeName: 'aws_s3_inbound', displayName: 'AWS S3 Bucket Reader', icon: '☁️', mode: 'pull',
                    description: 'Polls an S3 bucket prefix for new objects on a schedule. After downloading each object, can delete, move to another prefix, or tag it. Supports IAM role (ECS/EKS task role) or explicit access keys. Credentials are stored encrypted.',
                    notes: 'Requires a cron schedule. Use IAM role auth in AWS-hosted deployments — no keys needed.',
                    required: ['bucket_name', 'region'],
                    keyFields: [
                        { name: 'bucket_name', type: 'string', required: true, default: '—', notes: 'S3 bucket name.' },
                        { name: 'region', type: 'string', required: true, default: 'us-east-1', notes: 'AWS region, e.g. us-east-1.' },
                        { name: 'prefix', type: 'string', default: '—', notes: 'Folder prefix, e.g. hl7/inbound/. Include trailing slash.' },
                        { name: 'file_pattern', type: 'string', default: '*.hl7', notes: 'Glob to filter object keys.' },
                        { name: 'authentication_method', type: 'enum', default: 'access_keys', notes: 'access_keys | iam_role | assumed_role.' },
                        { name: 'access_key_id', type: 'string', default: '—', notes: 'AWS access key (only for access_keys method).' },
                        { name: 'secret_access_key', type: 'string (password)', default: '—', notes: 'AWS secret key — stored encrypted.' },
                        { name: 'after_processing', type: 'enum', default: 'move', notes: 'delete | move | tag | nothing.' },
                        { name: 'max_objects_per_poll', type: 'integer', default: '100', notes: 'Max S3 objects downloaded per cron run.' }
                    ],
                    example: { connectorType: 'aws_s3_inbound', config: { bucket_name: 'ehr-hl7-feeds', region: 'us-east-1', prefix: 'inbound/adt/', file_pattern: '*.hl7', authentication_method: 'iam_role', after_processing: 'move', max_objects_per_poll: 100 } }
                },
                {
                    typeName: 'azure_blob_inbound', displayName: 'Azure Blob Storage Reader', icon: '☁️', mode: 'pull',
                    description: 'Polls an Azure Blob Storage container for new blobs on a schedule. Supports account key or full connection string authentication. After downloading, can delete, move to an archive container, or tag the blob.',
                    notes: 'Requires a cron schedule. Use connection_string for simplicity in dev; use account_name + account_key in production.',
                    required: ['account_name', 'container_name'],
                    keyFields: [
                        { name: 'account_name', type: 'string', required: true, default: '—', notes: 'Azure Storage account name.' },
                        { name: 'account_key', type: 'string (password)', default: '—', notes: 'Storage account key — stored encrypted.' },
                        { name: 'connection_string', type: 'string (password)', default: '—', notes: 'Alternative to account_name+key. Full Azure connection string.' },
                        { name: 'container_name', type: 'string', required: true, default: '—', notes: 'Blob container name.' },
                        { name: 'prefix', type: 'string', default: '—', notes: 'Blob prefix / virtual folder.' },
                        { name: 'blob_pattern', type: 'string', default: '*.hl7', notes: 'Glob to filter blob names.' },
                        { name: 'after_processing', type: 'enum', default: 'move', notes: 'delete | move | tag | nothing.' },
                        { name: 'archive_container', type: 'string', default: '—', notes: 'Container to move blobs to after processing.' },
                        { name: 'max_blobs_per_poll', type: 'integer', default: '100', notes: 'Max blobs per cron run.' }
                    ],
                    example: { connectorType: 'azure_blob_inbound', config: { account_name: 'myehrstorage', account_key: '••••', container_name: 'hl7-inbound', prefix: 'adt/', blob_pattern: '*.hl7', after_processing: 'move', archive_container: 'hl7-processed', max_blobs_per_poll: 50 } }
                },
                {
                    typeName: 'gcs_inbound', displayName: 'Google Cloud Storage Reader', icon: '☁️', mode: 'pull',
                    description: 'Polls a GCS bucket for new objects on a schedule. Authenticates using a Google Cloud service account JSON key. After downloading, can delete or move objects to an archive bucket/prefix.',
                    notes: 'Requires a cron schedule. The service account needs storage.objects.get and storage.objects.list permissions.',
                    required: ['bucket_name', 'credentials_json'],
                    keyFields: [
                        { name: 'bucket_name', type: 'string', required: true, default: '—', notes: 'GCS bucket name.' },
                        { name: 'credentials_json', type: 'string (password)', required: true, default: '—', notes: 'Full service account JSON content — stored encrypted.' },
                        { name: 'prefix', type: 'string', default: '—', notes: 'Object prefix / folder, e.g. hl7/inbound/.' },
                        { name: 'file_pattern', type: 'string', default: '*.hl7', notes: 'Glob pattern to filter object names.' },
                        { name: 'after_processing', type: 'enum', default: 'move', notes: 'delete | move | nothing.' },
                        { name: 'archive_bucket', type: 'string', default: '—', notes: 'GCS bucket to copy processed objects to.' },
                        { name: 'archive_prefix', type: 'string', default: '—', notes: 'Prefix in the archive bucket.' },
                        { name: 'max_objects_per_poll', type: 'integer', default: '100', notes: 'Max objects per cron run.' }
                    ],
                    example: { connectorType: 'gcs_inbound', config: { bucket_name: 'ehr-hl7-bucket', credentials_json: '<paste GCP service account JSON here>', prefix: 'inbound/', file_pattern: '*.hl7', after_processing: 'move', archive_bucket: 'ehr-hl7-archive', max_objects_per_poll: 100 } }
                }
            ],
            parameters: [
                { name: 'connectorType', type: 'string', required: true, description: 'The listener type. tcp_mllp is the primary HL7 transport (also accepted as tcp_mllp_inbound). See Connector Type Reference section below for all supported types.' },
                { name: 'config', type: 'object', required: true, description: 'Connector-specific settings driven by the connector type. See sub-fields below.' },
                { name: 'config.host', type: 'string', required: false, description: 'Bind address for TCP/MLLP (default: 0.0.0.0 — all interfaces).' },
                { name: 'config.port', type: 'number', required: false, description: 'TCP port to listen on (default: 2575 — standard MLLP port).' },
                { name: 'config.tls_enabled', type: 'boolean', required: false, description: 'Enable TLS 1.2/1.3. Requires tls_cert_file and tls_key_file.' },
                { name: 'config.max_connections', type: 'number', required: false, description: 'Maximum simultaneous client connections (default: 100).' },
                { name: 'config.ack', type: 'object', required: false, description: 'ACK/NACK configuration for TCP/MLLP Inbound. Controls how the connector acknowledges received HL7 messages.' },
                { name: 'config.ack.mode', type: 'string', required: false, description: '"immediate" — send AA as soon as the message is queued (default). "none" — do not send any ACK (sender must not expect a response).' },
                { name: 'config.ack.on_error', type: 'string', required: false, description: '"suppress" — always send AA even on errors, handle failures internally (default). "nack" — send AE so the sender can retry when the queue is full or a critical error occurs.' },
                { name: 'config.ack.sending_app', type: 'string', required: false, description: 'MSH-3 in the generated ACK message (default: "ezHealthKonnect").' },
                { name: 'config.ack.sending_facility', type: 'string', required: false, description: 'MSH-4 in the generated ACK message (default: "EHK").' },
                { name: 'config.ack.text_success', type: 'string', required: false, description: 'MSA-3 text when sending AA (default: "Message received successfully").' },
                { name: 'config.ack.text_error', type: 'string', required: false, description: 'MSA-3 text when sending AE or AR (default: "Message processing error").' },
                { name: 'config.ack.script', type: 'string', required: false, description: 'Advanced: JavaScript function that fully overrides ACK logic. Must define buildACK(msg) returning { ackCode, textMessage }. Valid ackCode values: AA, AE, AR. Available on msg: controlID, messageType, sendingApp, sendingFacility, raw, defaultCode, defaultText. Errors fall back to the default ACK.' },
                { name: 'timeoutMs', type: 'number', required: false, description: 'Maximum wait time for data fetch in milliseconds (default: 30000). Not used for long-lived TCP listeners.' }
            ],
            bestPractices: [
                {
                    practice: 'Error Handling — configure per-step error handling on the connector.inbound step',
                    reason: 'If the inbound step itself fails (e.g. port already in use, bad config), the pipeline will not start. Setting onError: "suppress" lets the engine log the failure and continue trying to activate other steps.',
                    example: 'In the step properties, open "Error Handling & Retry" → set On Error = suppress, Default Value = {} to prevent pipeline abort on transient startup failures.'
                },
                {
                    practice: 'Retry — add retry config for pull-mode connectors (SFTP, S3, DB)',
                    reason: 'Pull connectors run on a schedule. If the remote server is temporarily unreachable, retry logic can transparently retry 2–3 times within the same poll window before failing.',
                    example: 'Error Handling & Retry → Enable Retry = on, Max Retries = 3, Delay = 5000 ms, Backoff Multiplier = 2 (exponential: 5s, 10s, 20s).'
                },
                {
                    practice: 'Error Handling — use onError: nack for TCP/MLLP ACK on critical errors',
                    reason: 'When a message causes a fatal downstream error (e.g. DB unavailable), sending AE (application error) or AR lets the HL7 sender retry rather than silently dropping the message.',
                    example: 'ACK tab → On Error = nack. The sender receives AE and can retry after its own retry interval.'
                },
                {
                    practice: 'ACK mode = none for fire-and-forget senders',
                    reason: 'Some legacy HL7 systems do not read ACK responses and will block the socket waiting for data that never matters. Setting mode=none closes the send side immediately.',
                    example: 'ACK tab → ACK Mode = none. No ACK message is sent; the MLLP session ends after receiving the message.'
                }
            ]
        };

        docs['connector.outbound'] = {
            description: 'Sends data to external systems via configurable outbound connectors. Supports TCP/MLLP, HTTP/REST, file writers, databases, message queues, and cloud storage.',
            useCases: [
                'Deliver transformed FHIR bundles to a REST endpoint',
                'Send HL7 messages to downstream systems via TCP/MLLP',
                'Write processed data to a database',
                'Archive messages to cloud storage (S3, Azure Blob, GCS)',
                'Publish events to Kafka or RabbitMQ'
            ],
            example: { connectorType: 'http_outbound', config: { url: 'https://fhir-server/api/Bundle', method: 'POST' }, contentField: 'transformed', contentType: 'application/fhir+json' },
            parameters: [
                { name: 'connectorType', type: 'string', required: true, description: 'The type of outbound connector (e.g., http_outbound, tcp_mllp_outbound, file_writer)' },
                { name: 'config', type: 'object', required: true, description: 'Connector-specific configuration (host, port, URL, credentials, etc.) - fields are driven by the connector type config_schema' },
                { name: 'contentField', type: 'string', required: false, description: 'Which field from the pipeline data to send (default: transformed)' },
                { name: 'contentType', type: 'string', required: false, description: 'Content type of the outgoing data (default: application/json)' }
            ]
        };

        // File Parser documentation
        docs['file_parser'] = {
            description: 'Parses structured files into an array of records for downstream pipeline steps. Supports CSV, TSV, fixed-width positional (CCLF, NACHA, X12), Excel .xlsx/.xls, Apache Avro, and Apache Parquet. Three source modes: read content from a pipeline field (field), from the server filesystem (local_path), or from a URI stored in a pipeline field — including s3://, https://, and file:// (field_as_path). Includes OOB healthcare templates for common fixed-width formats so no column mapping is needed. Typically placed after an Inbound Connector step, followed by Remove Duplicates and a Loop to process each record.',
            useCases: [
                'Parse a CSV feed received via SFTP — content stored in a pipeline field by the Inbound Connector',
                'Parse a fixed-width CCLF Part A file from a local volume mount using the cclf1 OOB template — no column mapping required',
                'Parse a NACHA ACH payment file using the nacha_entry OOB template',
                'Read a CSV from S3: a prior API step stores the S3 URI in a field, field_as_path fetches and parses it (credentials from interface connectivity config)',
                'Parse an Excel .xlsx spreadsheet received as base64 content in a pipeline field',
                'Parse Apache Avro files from Kafka consumers or data pipelines — column names come from the embedded Avro schema',
                'Parse Apache Parquet exports from data warehouses (Snowflake, BigQuery, Databricks) — supports files up to 500 MB',
                'Batch mode: parse all CCLF files in a directory matching a glob pattern and combine into one record array',
                'Auto-detect format: drop any file and let magic bytes and heuristics determine CSV/TSV/XLSX/Avro/Parquet automatically',
                'Sample a large CSV with maxRecords: 100 — streaming read, only 100 rows loaded into memory regardless of file size'
            ],
            example: {
                sourceType: 'field',
                sourceField: 'enriched.connector_result.content',
                autoDetect: true
            },
            parameters: [
                {
                    name: 'sourceType',
                    type: 'enum: field | local_path | field_as_path',
                    required: false,
                    description: 'Where to read the file from. "field" (default) — raw file content already in a pipeline field (set by an Inbound Connector). "local_path" — read directly from the server/container filesystem; supports glob pattern + batch mode. "field_as_path" — a pipeline field holds a URI that is resolved at runtime: s3://bucket/key (AWS S3, credentials from interface connectivity config), https://... (HTTP GET), file:///... (local filesystem).'
                },
                {
                    name: 'sourceField',
                    type: 'string',
                    required: true,
                    description: 'For sourceType=field: the pipeline field holding raw file content (e.g. "enriched.sftp_content"). For sourceType=field_as_path: the pipeline field holding the URI to resolve (e.g. "enriched.s3_uri").'
                },
                {
                    name: 'filePath',
                    type: 'string',
                    required: false,
                    description: 'For sourceType=local_path: absolute path to the file or directory on the server/container (e.g. "/data/cclf/PARTA.T.ACO.D250101.T000001"). With batchMode=true, this is the base directory and filePattern is the glob.'
                },
                {
                    name: 'batchMode',
                    type: 'boolean',
                    required: false,
                    description: 'local_path only. When true, processes all files matching filePattern in the filePath directory and returns a combined array of results with per-file record counts. Use for daily batch feeds where an entire directory needs to be processed.'
                },
                {
                    name: 'filePattern',
                    type: 'string',
                    required: false,
                    description: 'Glob filename pattern used in batch mode (e.g. "PARTA*.T.*"). Combined with filePath to produce the full glob. Example: filePath="/data/cclf" + filePattern="PARTA*.T.*" matches all Part A files in the directory.'
                },
                {
                    name: 'autoDetect',
                    type: 'boolean',
                    required: false,
                    description: 'Enable automatic format detection. Magic bytes are checked first (detects XLSX, XLS, Avro, Parquet from binary headers). Then file extension (for local_path). Then delimiter heuristics (CSV vs TSV). Also auto-infers hasHeader from column name patterns. Overrides fileFormat.'
                },
                {
                    name: 'fileFormat',
                    type: 'enum',
                    required: false,
                    description: 'Explicit format: csv, tsv, fixed_width, xlsx, xls, avro, parquet, auto. Binary formats (xlsx, xls, avro, parquet) are auto-detected from magic bytes when sourceType=local_path, overriding whatever is configured. Avro and Parquet derive column names from their embedded schema — hasHeader is ignored for these.'
                },
                {
                    name: 'template',
                    type: 'string',
                    required: false,
                    description: 'OOB template key for fixed-width formats — no manual column mapping needed. Available templates: cclf1 (Part A Claims Header), cclf2 (Part A Revenue), cclf3 (Part A PPS/SNF), cclf4 (Part B Physicians), cclf5 (Part B DME), cclf6 (Part D Drug Events), cclf7 (Beneficiary Demographics), cclf8 (Beneficiary XREF), nacha_entry (ACH Entry Detail), era_835_header (X12 835 Interchange).'
                },
                {
                    name: 'columns',
                    type: 'array',
                    required: false,
                    description: 'Manual column definitions for fixed-width format. Each entry: { name, start, length }. Start is 1-based byte position, length is field width in bytes. Overrides template if both are set.'
                },
                {
                    name: 'delimiter',
                    type: 'string',
                    required: false,
                    description: 'Field separator character for CSV/TSV. Default: comma for CSV, tab for TSV. Ignored for binary formats (xlsx, xls, avro, parquet).'
                },
                {
                    name: 'hasHeader',
                    type: 'boolean',
                    required: false,
                    description: 'Whether the first row contains column names. Default: true. When false, columns are named col_1, col_2, ... for CSV/TSV. Ignored for Avro and Parquet (schema is embedded).'
                },
                {
                    name: 'sheetName',
                    type: 'string',
                    required: false,
                    description: 'xlsx/xls: Name of the sheet to parse. Leave empty to use the first sheet. Use sheetIndex if the sheet has no name.'
                },
                {
                    name: 'sheetIndex',
                    type: 'number',
                    required: false,
                    description: 'xlsx/xls: 0-based sheet index. Used when sheetName is empty. Default: 0 (first sheet).'
                },
                {
                    name: 'contentEncoding',
                    type: 'enum',
                    required: false,
                    description: 'Set to "base64" when binary file content (Excel, Avro, Parquet) was base64-encoded before being stored in a pipeline field. Common when binary data passes through JSON-based APIs or message queues. The executor decodes it before parsing.'
                },
                {
                    name: 'trimFields',
                    type: 'boolean',
                    required: false,
                    description: 'Trim leading/trailing whitespace from all string values. Default: true. Applies to CSV, TSV, fixed-width, and string fields in Avro/Parquet.'
                },
                {
                    name: 'skipRows',
                    type: 'number',
                    required: false,
                    description: 'Number of rows/records to skip from the top before parsing begins. Useful for files with non-data header rows (e.g. report title, generation date). For Avro/Parquet: skips that many records in the stream.'
                },
                {
                    name: 'maxRecords',
                    type: 'number',
                    required: false,
                    description: 'Maximum records to parse (0 = unlimited). For CSV/TSV: uses streaming when set — only maxRecords rows are read from the file, making it O(maxRecords) memory regardless of file size. Ideal for sampling large files. For Avro/Parquet: stops reading after maxRecords records.'
                },
                {
                    name: 'maxFileSizeMB',
                    type: 'number',
                    required: false,
                    description: 'File size limit in MB for local_path and file:// sources (0 = default 100 MB, hard cap 500 MB). The file size is checked via stat() before reading — oversized files are rejected immediately with a descriptive error. Use maxRecords to sample files larger than the limit.'
                },
                {
                    name: 'interface_id',
                    type: 'string',
                    required: false,
                    description: 'Required when sourceType=field_as_path and the URI is an s3:// address. Identifies which interface connectivity config to look up for AWS credentials (access key ID, secret access key, region). Credentials are AES-256-GCM decrypted at runtime — never stored in plaintext in the step config.'
                }
            ],
            examples: [
                {
                    label: 'CSV from pipeline field (SFTP content)',
                    config: { sourceType: 'field', sourceField: 'enriched.sftp_content', fileFormat: 'csv', hasHeader: true, trimFields: true }
                },
                {
                    label: 'CCLF1 fixed-width from local file (OOB template)',
                    config: { sourceType: 'local_path', filePath: '/data/cclf/PARTA.T.A0001.ACO.ZC1Y24.D250101.T000001', fileFormat: 'fixed_width', template: 'cclf1' }
                },
                {
                    label: 'NACHA ACH payment file (OOB template)',
                    config: { sourceType: 'local_path', filePath: '/data/nacha/ACH20260201.txt', fileFormat: 'fixed_width', template: 'nacha_entry' }
                },
                {
                    label: 'S3 file via URI in pipeline field',
                    config: { sourceType: 'field_as_path', sourceField: 'enriched.s3_uri', fileFormat: 'csv', hasHeader: true, interface_id: 'your-interface-id' }
                },
                {
                    label: 'Excel from pipeline field (base64-encoded)',
                    config: { sourceType: 'field', sourceField: 'enriched.excel_b64', fileFormat: 'xlsx', contentEncoding: 'base64', sheetName: 'Claims', hasHeader: true }
                },
                {
                    label: 'Apache Avro — auto-detect format',
                    config: { sourceType: 'field', sourceField: 'enriched.avro_bytes', autoDetect: true }
                },
                {
                    label: 'Apache Parquet from local file with size limit',
                    config: { sourceType: 'local_path', filePath: '/data/warehouse/claims_2026.parquet', fileFormat: 'parquet', maxRecords: 5000, maxFileSizeMB: 200 }
                },
                {
                    label: 'Batch: all CCLF1 files in a directory',
                    config: { sourceType: 'local_path', filePath: '/data/cclf/', filePattern: 'PARTA*.T.*.ZC1*.T*', fileFormat: 'fixed_width', template: 'cclf1', batchMode: true }
                },
                {
                    label: 'Sample first 100 rows from a large CSV',
                    config: { sourceType: 'local_path', filePath: '/data/large_feed.csv', fileFormat: 'csv', hasHeader: true, maxRecords: 100 }
                }
            ],
            oobTemplates: [
                { key: 'cclf1', name: 'CCLF1 — Part A Claims Header', note: 'CMS Medicare claims, Part A inpatient' },
                { key: 'cclf2', name: 'CCLF2 — Part A Claims Revenue', note: 'Revenue center detail lines' },
                { key: 'cclf3', name: 'CCLF3 — Part A PPS / SNF', note: 'Prospective Payment System / Skilled Nursing' },
                { key: 'cclf4', name: 'CCLF4 — Part B Physicians', note: 'Physician and supplier claims' },
                { key: 'cclf5', name: 'CCLF5 — Part B DME', note: 'Durable Medical Equipment claims' },
                { key: 'cclf6', name: 'CCLF6 — Part D Drug Events', note: 'Prescription drug events' },
                { key: 'cclf7', name: 'CCLF7 — Beneficiary Demographics', note: 'Patient demographic data' },
                { key: 'cclf8', name: 'CCLF8 — Beneficiary XREF', note: 'Beneficiary cross-reference' },
                { key: 'nacha_entry', name: 'NACHA ACH Entry Detail', note: 'ACH payment record (94-char fixed)' },
                { key: 'era_835_header', name: 'ERA 835 Interchange Header', note: 'X12 835 remittance advice' }
            ]
        };

        // Remove Duplicates documentation
        docs['remove_duplicates'] = {
            description: 'Removes duplicate records from an array field using configurable key fields and merge strategies. ' +
                'Supports full-record hashing (no key fields needed), multi-field composite keys, three merge strategies, ' +
                'case-insensitive matching, and flexible handling of records with missing keys.\n\n' +
                'Source arrays come from prior steps via the steps.{namespace}.step_output path — ' +
                'for example a File Parser step produces steps.{ns}.step_output.records, ' +
                'a Database Enrichment step produces steps.{ns}.step_output.rows. ' +
                'Use the "Detect fields" button in the UI to auto-populate key field candidates.',
            useCases: [
                'Remove duplicate patient rows parsed from a CSV or CCLF file (key: patient_mrn)',
                'Dedup FHIR bundle entries by resource.id before loading into a FHIR server',
                'Dedup claims rows from an 835/837 file by claim_id + line_num',
                'Dedup HL7 OBX observations by test_code + observation_date within one message',
                'Merge partial records — keep first occurrence, backfill missing fields from later duplicates (strategy: merge)',
                'Drop records with null patient_id (nullKeyBehavior: remove) before FHIR mapping',
                'Deduplicate API enrichment results that return the same record multiple times'
            ],
            example: {
                sourceField: 'steps.file_parser_a1b2c3.step_output.records',
                keyFields: ['patient_id', 'visit_date'],
                strategy: 'first',
                caseSensitive: false,
                nullKeyBehavior: 'remove'
            },
            examples: [
                {
                    label: 'Dedup CSV records by patient ID (keep first)',
                    config: {
                        sourceField: 'steps.file_parser_a1b2c3.step_output.records',
                        keyFields: ['patient_id'],
                        strategy: 'first',
                        caseSensitive: false,
                        nullKeyBehavior: 'remove'
                    }
                },
                {
                    label: 'Dedup claims by claim ID + line number (keep last = most recent correction)',
                    config: {
                        sourceField: 'steps.file_parser_a1b2c3.step_output.records',
                        keyFields: ['claim_id', 'line_num'],
                        strategy: 'last',
                        caseSensitive: true,
                        nullKeyBehavior: 'group'
                    }
                },
                {
                    label: 'Merge partial lab records — backfill missing fields from later duplicates',
                    config: {
                        sourceField: 'steps.script_enrichment_b9fb33.step_output.observations',
                        keyFields: ['test_code', 'obs_date'],
                        strategy: 'merge',
                        caseSensitive: false,
                        nullKeyBehavior: 'keep'
                    }
                },
                {
                    label: 'Dedup database rows, write to new field (preserve original)',
                    config: {
                        sourceField: 'steps.database_enrichment_c3d4e5.step_output.rows',
                        keyFields: ['member_id'],
                        strategy: 'first',
                        caseSensitive: false,
                        outputField: 'deduped_members'
                    }
                },
                {
                    label: 'Full-record strict dedup — no key fields (every field must match)',
                    config: {
                        sourceField: 'steps.api_enrichment_f1e2d3.step_output.results',
                        strategy: 'first',
                        caseSensitive: true,
                        nullKeyBehavior: 'group'
                    }
                }
            ],
            parameters: [
                {
                    name: 'sourceField',
                    type: 'string',
                    required: true,
                    description: 'Dot-path to the array to deduplicate. ' +
                        'Must point to an array produced by a prior step — ' +
                        'File Parser: steps.{ns}.step_output.records; ' +
                        'Database Enrichment: steps.{ns}.step_output.rows; ' +
                        'Script Enrichment: steps.{ns}.step_output.{yourField}. ' +
                        'Use the Source Step picker in the UI to auto-build this path.'
                },
                {
                    name: 'keyFields',
                    type: 'Array<string>',
                    required: false,
                    description: 'Field names within each record that form the unique key. ' +
                        'Supports dot-paths for nested fields (e.g. "patient.id"). ' +
                        'Leave empty to hash the entire record — every field must match for a record to count as a duplicate. ' +
                        'Use the "Detect fields" button to auto-populate from upstream step output.'
                },
                {
                    name: 'strategy',
                    type: 'enum: first | last | merge',
                    required: false,
                    description: '"first" (default) — keep the earliest occurrence, discard all later duplicates. ' +
                        '"last" — keep the most recent occurrence (useful when later records are corrections). ' +
                        '"merge" — keep the first record and non-destructively backfill any absent fields from later records.'
                },
                {
                    name: 'caseSensitive',
                    type: 'boolean',
                    required: false,
                    description: 'When false, key field values are lowercased before hashing so "SMITH" = "smith". Default: true.'
                },
                {
                    name: 'nullKeyBehavior',
                    type: 'enum: group | keep | remove',
                    required: false,
                    description: '"group" (default) — records with null/absent key fields share one dedup bucket. ' +
                        '"keep" — records with missing keys bypass dedup and are always kept. ' +
                        '"remove" — records with null or absent key fields are dropped entirely.'
                },
                {
                    name: 'outputField',
                    type: 'string',
                    required: false,
                    description: 'Write the deduplicated array to this field instead of updating sourceField in-place. ' +
                        'Leave empty to update the source array directly. ' +
                        'Example: "deduped_patients" — downstream steps access it as steps.{thisStep}.step_output.result_path.'
                },
                {
                    name: 'previewLimit',
                    type: 'number',
                    required: false,
                    description: 'Maximum records to include in the step_output.records preview (default 100, max 1000). ' +
                        'The full deduplicated array is always written to outputField/sourceField regardless of this setting.'
                }
            ],
            stepOutput: {
                description: 'The full deduplicated array is written to outputField (or sourceField when outputField is blank). ' +
                    'The following variables are also available in downstream steps via steps.{ns}.step_output:',
                fields: [
                    { name: 'result_path', type: 'string', description: 'Dot-path where the full deduplicated array was written (use this in downstream sourceField configs)' },
                    { name: 'records', type: 'array', description: 'Preview of up to previewLimit (default 100) deduplicated records — visible in the test panel' },
                    { name: 'records_truncated', type: 'boolean', description: 'true when the full result has more rows than the preview limit' },
                    { name: 'original_count', type: 'number', description: 'Total records in the source array before dedup' },
                    { name: 'dedup_count', type: 'number', description: 'Records in the deduplicated output' },
                    { name: 'removed_count', type: 'number', description: 'Duplicate records discarded' },
                    { name: 'null_key_kept', type: 'number', description: 'Records with missing keys that were kept (nullKeyBehavior=keep)' },
                    { name: 'null_key_removed', type: 'number', description: 'Records with missing keys that were dropped (nullKeyBehavior=remove)' }
                ]
            }
        };

        // Add aliases for renamed step types (layer prefix removed)
        docs['enrichment.script'] = docs['pre.enrichment.script'];
        docs['enrichment.api'] = docs['pre.enrichment.api'];
        docs['enrichment.database'] = docs['pre.enrichment.database'];
        docs['field_validation'] = docs['pre.validation'];
        docs['hl7_fhir_transform'] = docs['core.mapping'];
        docs['field_mapping'] = docs['core.transformation'];
        docs['fhir_validation'] = docs['post.validation'];

        // Data Masking / Anonymization documentation
        docs['data_masking'] = {
            description:
                'Masks or anonymizes sensitive PHI/PII fields for HIPAA compliance. Applies ordered masking ' +
                'rules to individual fields using six strategies:\n\n' +
                '  • mask — replace every character with maskChar (default *)\n' +
                '  • redact — replace with [REDACTED]\n' +
                '  • partial — reveal first N and/or last N characters, mask the middle\n' +
                '  • hash — SHA-256 deterministic 16-char hex (same salt = same output = join-safe de-ID)\n' +
                '  • tokenize — non-reversible TOK-{12hex} token\n' +
                '  • substitute — replace with realistic fake data (name, SSN, phone, email, date, address, or custom fixed value)\n\n' +
                'Field paths are format-agnostic — any of these work:\n' +
                '  • HL7 v2:  PID.5, PID.19, MSH.9.1\n' +
                '  • FHIR R4: steps.hl7_fhir_transform.step_output.fhir_bundle.entry[0].resource.patient.name[0].family\n' +
                '    (entry index is auto-resolved — if the Patient is at entry[1], it will be found automatically)\n' +
                '  • Cross-step JSON: steps.api_enrichment.step_output.email\n' +
                '  • CSV / DB records: steps.file_parser.step_output.records[0].ssn\n' +
                '    (index [0] is auto-searched across all records in the array)\n\n' +
                'Enable "Mask All PHI" to instantly apply pre-configured HIPAA Safe Harbor rules for the ' +
                'selected format (HL7 v2: 23 rules / FHIR R4: 20 rules / JSON: 18 rules). Custom rules run after.\n\n' +
                'Step output is available downstream via steps.{namespace}.step_output.*',

            useCases: [
                'Mask HL7 patient name (PID.5) and DOB (PID.7) before sending to a test environment',
                'Hash SSN (PID.19) for de-identification while keeping cross-dataset join capability',
                'Partial-mask phone — keep last 4 digits: 555-867-5309 → ***-***-5309 (preserveFormat)',
                'Substitute patient name with realistic fake name for synthetic test data generation',
                'Mask FHIR Patient.name.family after an HL7→FHIR transform step — entry index resolved automatically',
                'Mask email returned by an API enrichment step: steps.api_enrichment.step_output.email',
                'Mask SSN column in all CSV rows from a File Parser step: steps.file_parser.step_output.records[0].ssn',
                'Enable Mask All PHI → Format = FHIR to auto-mask 20 FHIR R4 PHI fields with one toggle',
                'Redact all PHI before storing to audit log (maskAllPHI: true, strategy override: redact)',
                'Use preserveFormat with partial to keep SSN dashes: 123-45-6789 → ***-**-6789'
            ],

            example: {
                maskAllPHI: false,
                maskAllPHIFormat: 'hl7v2',
                preserveFormat: false,
                rules: [
                    { field: 'PID.5',  strategy: 'mask' },
                    { field: 'PID.19', strategy: 'hash', hashSalt: 'myOrgSecret2025' },
                    { field: 'PID.13', strategy: 'partial', keepFirst: 0, keepLast: 4 },
                    { field: 'PID.5',  strategy: 'substitute', substituteType: 'name' },
                    {
                        field: 'steps.hl7_fhir_transform.step_output.fhir_bundle.entry[0].resource.patient.name[0].family',
                        strategy: 'mask'
                    },
                    { field: 'steps.api_enrichment.step_output.email', strategy: 'mask' }
                ]
            },

            examples: [
                {
                    label: 'HL7 — full mask patient name',
                    config: { rules: [{ field: 'PID.5', strategy: 'mask' }], maskAllPHI: false }
                },
                {
                    label: 'HL7 — hash SSN for join-safe de-identification',
                    config: {
                        rules: [{ field: 'PID.19', strategy: 'hash', hashSalt: 'myOrgSecret2025' }],
                        maskAllPHI: false
                    }
                },
                {
                    label: 'HL7 — partial-mask phone, keep last 4, preserve dashes',
                    config: {
                        rules: [{ field: 'PID.13', strategy: 'partial', keepFirst: 0, keepLast: 4 }],
                        maskAllPHI: false,
                        preserveFormat: true
                    }
                },
                {
                    label: 'HL7 — substitute patient name with realistic fake name',
                    config: {
                        rules: [{ field: 'PID.5', strategy: 'substitute', substituteType: 'name' }],
                        maskAllPHI: false
                    }
                },
                {
                    label: 'FHIR — mask family name from HL7→FHIR transform step output',
                    config: {
                        rules: [{
                            field: 'steps.hl7_fhir_transform.step_output.fhir_bundle.entry[0].resource.patient.name[0].family',
                            strategy: 'mask'
                        }],
                        maskAllPHI: false
                    }
                },
                {
                    label: 'FHIR — auto-mask all 20 FHIR PHI fields (Mask All PHI)',
                    config: { rules: [], maskAllPHI: true, maskAllPHIFormat: 'fhir' }
                },
                {
                    label: 'Cross-step — mask email from API enrichment result',
                    config: {
                        rules: [{ field: 'steps.api_enrichment.step_output.email', strategy: 'mask' }],
                        maskAllPHI: false
                    }
                },
                {
                    label: 'CSV / DB records — hash SSN in all records from File Parser',
                    config: {
                        rules: [{ field: 'steps.file_parser.step_output.records[0].ssn', strategy: 'hash', hashSalt: 'myOrgSecret2025' }],
                        maskAllPHI: false
                    }
                },
                {
                    label: 'Auto-mask all HL7 HIPAA PHI — 23 fields, 13 identifiers',
                    config: { rules: [], maskAllPHI: true, maskAllPHIFormat: 'hl7v2' }
                },
                {
                    label: 'Substitute DOB with realistic fake date (preserve year)',
                    config: {
                        rules: [{ field: 'PID.7', strategy: 'substitute', substituteType: 'date' }],
                        maskAllPHI: false
                    }
                }
            ],

            parameters: [
                {
                    name: 'rules',
                    type: 'MaskingRule[]',
                    required: false,
                    description: 'Ordered list of field masking rules. Each rule specifies a field path and strategy. ' +
                        'Rules run in sequence. If multiple rules match the same field, the last one applied wins.'
                },
                {
                    name: 'rules[].field',
                    type: 'string',
                    required: true,
                    description: 'Field path to mask. Supports all formats:\n' +
                        '  HL7 v2: PID.5, PID.19, MSH.9.1, PID.5.1\n' +
                        '  FHIR R4: steps.{step}.step_output.fhir_bundle.entry[0].resource.patient.name[0].family\n' +
                        '    (entry[N] index is auto-searched if the field is not found at index N)\n' +
                        '  Cross-step JSON: steps.{step}.step_output.email\n' +
                        '  CSV / DB records: steps.{step}.step_output.records[0].field_name\n' +
                        '    (array index is auto-searched across all records)\n' +
                        '  Generic JSON: data.patient.ssn, message.payload.name'
                },
                {
                    name: 'rules[].strategy',
                    type: 'enum',
                    required: true,
                    description: 'Masking strategy:\n' +
                        '  "mask"       — replace all chars with maskChar (default *). Example: John → ****\n' +
                        '  "redact"     — replace with [REDACTED]. Example: John → [REDACTED]\n' +
                        '  "partial"    — keep keepFirst chars at start and keepLast chars at end, mask middle.\n' +
                        '                 Example: 555-867-5309 with keepLast=4 → 555-867-****\n' +
                        '                 Set preserveFormat=true to also keep non-digit separators in place.\n' +
                        '  "hash"       — SHA-256 of (hashSalt + value), returns first 16 hex chars.\n' +
                        '                 Deterministic: same input + same salt = same output (join-safe de-ID).\n' +
                        '  "tokenize"   — non-reversible token: TOK-{12 hex chars}. Each unique value gets a unique token.\n' +
                        '  "substitute" — replace with realistic fake data. Set substituteType to control output:\n' +
                        '                 name, ssn (9XX-XX-XXXX ITIN range), phone ((555) XXX-XXXX),\n' +
                        '                 email (test.{hex}@example-test.com), date (preserves year),\n' +
                        '                 address (1000-9999 fictional street), custom (fixed substituteValue).\n' +
                        '                 All substitute values are deterministic — same input = same fake output.'
                },
                {
                    name: 'rules[].substituteType',
                    type: 'enum',
                    required: false,
                    description: 'Category of fake data to generate (substitute strategy only). Options: ' +
                        'name, ssn, phone, email, date, address, custom. ' +
                        'All types produce values that are clearly non-real and safe for test environments.'
                },
                {
                    name: 'rules[].substituteValue',
                    type: 'string',
                    required: false,
                    description: 'Fixed replacement string used when substituteType is "custom". ' +
                        'Example: substituteValue: "[TEST PATIENT]". The same value is used for every field match.'
                },
                {
                    name: 'rules[].maskChar',
                    type: 'string (1 char)',
                    required: false,
                    description: 'Replacement character for mask and partial strategies. Default: *.'
                },
                {
                    name: 'rules[].keepFirst',
                    type: 'integer',
                    required: false,
                    description: 'Characters to reveal at the start of the value (partial strategy). Default: 0.'
                },
                {
                    name: 'rules[].keepLast',
                    type: 'integer',
                    required: false,
                    description: 'Characters to reveal at the end of the value (partial strategy). Default: 4.'
                },
                {
                    name: 'rules[].hashSalt',
                    type: 'string',
                    required: false,
                    description: 'Salt prepended before SHA-256 hashing (hash strategy). Default: "ezHealthKonnect". ' +
                        'Use the same salt across environments to preserve cross-dataset join capability on hashed values.'
                },
                {
                    name: 'rules[].pattern',
                    type: 'string (regex)',
                    required: false,
                    description: 'Optional regex. When set, only substrings matching the pattern are masked — ' +
                        'non-matching characters are preserved. Example: \\d{4} on "Acct 1234-5678" masks only the digits.'
                },
                {
                    name: 'maskAllPHI',
                    type: 'boolean',
                    required: false,
                    description: 'When true, prepends format-specific HIPAA Safe Harbor rules before any custom rules:\n' +
                        '  HL7 v2 (23 rules): Names (PID.5/6, NK1.2, GT1.3), Geographic (PID.11.x), Dates (PID.7/29, PV1.44/45),\n' +
                        '    Phone/Fax (PID.13/14), Email (PID.13.4), SSN (PID.19 hash), MRN (PID.3 partial),\n' +
                        '    Insurance (IN1.49/2), Account (PID.18), License (PID.20 hash), Device ID (OBX.18 hash), Other IDs (PID.2/4)\n' +
                        '  FHIR R4 (20 rules): Patient.name, Patient.address, Patient.birthDate, Patient.telecom,\n' +
                        '    Patient.identifier, Encounter.period, Coverage, Account, Practitioner, Device\n' +
                        '  JSON (18 rules): patient.name/firstName/lastName, patient.dateOfBirth/dob, patient.ssn,\n' +
                        '    patient.phone/fax, patient.email, patient.mrn, patient.address, patient.zipCode, patient.insuranceId\n' +
                        'Custom rules still apply after the auto-PHI rules.'
                },
                {
                    name: 'maskAllPHIFormat',
                    type: 'enum',
                    required: false,
                    description: 'Format for auto-PHI rules when maskAllPHI is true. Options: "hl7v2" (default), "fhir", "json". ' +
                        'Set to "fhir" when masking data after an HL7→FHIR transform step, ' +
                        'or "json" when masking generic JSON patient records from a DB or CSV source.'
                },
                {
                    name: 'preserveFormat',
                    type: 'boolean',
                    required: false,
                    description: 'For partial strategy: mask digit characters only and keep non-digit separators in place. ' +
                        'Example: 555-867-5309 with keepLast=4 → ***-***-5309. ' +
                        'keepFirst/keepLast count only digits — separators are never counted or masked.'
                }
            ],

            stepOutput: {
                description: 'Reference these in downstream steps via steps.{namespace}.step_output.*',
                fields: [
                    { name: 'masked_count',  type: 'integer',  description: 'Number of fields successfully masked. A field not found in the message is not counted.' },
                    { name: 'masked_fields', type: 'string[]', description: 'Ordered list of resolved field paths that were masked (may differ from the configured path when entry index was auto-resolved).' },
                    { name: 'total_rules',   type: 'integer',  description: 'Total rules evaluated including maskAllPHI auto-rules.' }
                ]
            }
        };
        docs['post.data_masking'] = docs['data_masking'];

        // Default documentation for unknown step types
        return docs[stepType] || {
            description: `Configuration for ${stepType} step. This is a custom step type.`,
            useCases: ['Custom transformation logic', 'Specialized data processing'],
            example: { config: 'Custom configuration' },
            parameters: []
        };
    }

    // ─── AI Tab ────────────────────────────────────────────────────────────────

    /**
     * Renders the AI assistant tab inside the step properties modal.
     * Provides: Explain this step, Generate script (script steps only), Ask a question.
     */
    setupAITab(step, container, isPreview) {
        const isScriptStep = ['enrichment.script', 'pre.enrichment.script'].includes(step.stepType);
        const pipeline = this.builder?.pipeline || {};
        const ctx = {
            step_id:      step.id      || '',
            pipeline_id:  pipeline.id  || '',
            interface_id: pipeline.interfaceId || '',
            message_type: pipeline.messageType || '',
            page:         'pipeline-builder'
        };

        const esc = s => String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');

        container.innerHTML = `
<div style="display:flex;flex-direction:column;gap:16px;padding:4px 0;">

  <!-- Explain this step -->
  <div style="border:1px solid #e2e8f0;border-radius:10px;overflow:hidden;">
    <div style="background:#f8fafc;padding:10px 14px;display:flex;align-items:center;gap:8px;border-bottom:1px solid #e2e8f0;">
      <span style="font-size:15px;">💡</span>
      <strong style="font-size:13px;color:#1e3a8a;">Explain this step</strong>
      ${isPreview ? '' : `<button id="ai-tab-explain-btn" style="margin-left:auto;background:linear-gradient(135deg,#f472b6,#1e3a8a);color:#fff;border:none;padding:5px 14px;border-radius:20px;cursor:pointer;font-size:12px;">Explain</button>`}
    </div>
    <div id="ai-tab-explain-out" style="padding:12px 14px;font-size:13px;color:#4b5563;min-height:40px;line-height:1.6;">
      ${isPreview ? '<em style="color:#9ca3af;">Save the step first to explain it.</em>' : '<em style="color:#9ca3af;">Click Explain to get a plain-English description of this step.</em>'}
    </div>
  </div>

  ${isScriptStep && !isPreview ? `
  <!-- Generate script -->
  <div style="border:1px solid #e2e8f0;border-radius:10px;overflow:hidden;">
    <div style="background:#f8fafc;padding:10px 14px;display:flex;align-items:center;gap:8px;border-bottom:1px solid #e2e8f0;">
      <span style="font-size:15px;">⚙️</span>
      <strong style="font-size:13px;color:#1e3a8a;">Generate script</strong>
    </div>
    <div style="padding:12px 14px;">
      <textarea id="ai-tab-script-desc" placeholder="Describe what the script should do…&#10;e.g. Extract patient name from PID.5.1 and PID.5.2, combine as 'Last, First', add to output.patientName"
        style="width:100%;height:80px;border:1px solid #e2e8f0;border-radius:8px;padding:9px 12px;font-size:12.5px;font-family:inherit;resize:vertical;outline:none;color:#1a202c;box-sizing:border-box;"></textarea>
      <div style="display:flex;gap:8px;margin-top:8px;">
        <button id="ai-tab-gen-btn" style="background:linear-gradient(135deg,#f472b6,#1e3a8a);color:#fff;border:none;padding:7px 18px;border-radius:20px;cursor:pointer;font-size:12px;">Generate</button>
        <button id="ai-tab-insert-btn" style="display:none;background:#1e3a8a;color:#fff;border:none;padding:7px 18px;border-radius:20px;cursor:pointer;font-size:12px;">Insert into editor</button>
      </div>
      <div id="ai-tab-gen-preview" style="display:none;margin-top:10px;background:#0f172a;border-radius:8px;padding:12px 14px;font-size:12px;font-family:'Fira Code',Consolas,monospace;color:#e2e8f0;white-space:pre-wrap;max-height:300px;overflow-y:auto;"></div>
    </div>
  </div>
  ` : ''}

  <!-- Ask a question -->
  <div style="border:1px solid #e2e8f0;border-radius:10px;overflow:hidden;">
    <div style="background:#f8fafc;padding:10px 14px;display:flex;align-items:center;gap:8px;border-bottom:1px solid #e2e8f0;">
      <span style="font-size:15px;">✏️</span>
      <strong style="font-size:13px;color:#1e3a8a;">Ask ezCompanion about this step</strong>
    </div>
    <div style="padding:12px 14px;">
      <div style="display:flex;flex-wrap:wrap;gap:6px;margin-bottom:10px;">
        <button class="ai-tab-quick" data-q="What data does this step output?" style="background:#f0f4ff;border:1px solid #c7d2fe;color:#3730a3;padding:4px 11px;border-radius:16px;cursor:pointer;font-size:11.5px;">What does it output?</button>
        <button class="ai-tab-quick" data-q="What could cause this step to fail?" style="background:#fff0f5;border:1px solid #fbcfe8;color:#9d174d;padding:4px 11px;border-radius:16px;cursor:pointer;font-size:11.5px;">Why might it fail?</button>
        <button class="ai-tab-quick" data-q="How do I configure this step correctly?" style="background:#f0fdf4;border:1px solid #bbf7d0;color:#166534;padding:4px 11px;border-radius:16px;cursor:pointer;font-size:11.5px;">How to configure?</button>
      </div>
      <button id="ai-tab-open-companion" style="width:100%;background:#f8fafc;border:1px dashed #c7d2fe;color:#1e3a8a;padding:9px;border-radius:8px;cursor:pointer;font-size:12.5px;font-weight:500;">
        Open ezCompanion with this step in context →
      </button>
    </div>
  </div>

</div>`;

        // Wire: Explain button
        const explainBtn = container.querySelector('#ai-tab-explain-btn');
        const explainOut = container.querySelector('#ai-tab-explain-out');
        if (explainBtn && step.id) {
            explainBtn.addEventListener('click', async () => {
                explainBtn.disabled = true;
                explainBtn.textContent = '…';
                explainOut.innerHTML = '<em style="color:#9ca3af;">Thinking…</em>';
                let full = '';
                try {
                    await window.AIAssistant.explainStep(step.id, (_tok, text) => {
                        full = text;
                        explainOut.innerHTML = full
                            .replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;')
                            .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
                            .replace(/`([^`]+)`/g, '<code style="background:#f1f5f9;padding:1px 4px;border-radius:3px;font-size:11px">$1</code>')
                            .replace(/\n/g, '<br>');
                    });
                } catch (e) {
                    explainOut.innerHTML = `<span style="color:#dc2626;">⚠️ ${esc(e.message)}</span>`;
                } finally {
                    explainBtn.disabled = false;
                    explainBtn.textContent = 'Explain';
                }
            });
        }

        // Wire: Generate script
        if (isScriptStep && !isPreview) {
            const genBtn     = container.querySelector('#ai-tab-gen-btn');
            const insertBtn  = container.querySelector('#ai-tab-insert-btn');
            const descArea   = container.querySelector('#ai-tab-script-desc');
            const preview    = container.querySelector('#ai-tab-gen-preview');
            let   lastScript = '';

            genBtn.addEventListener('click', async () => {
                const desc = descArea.value.trim();
                if (!desc) { descArea.focus(); return; }

                genBtn.disabled = true;
                genBtn.textContent = 'Generating…';
                insertBtn.style.display = 'none';
                preview.style.display = 'block';
                preview.textContent = '';
                lastScript = '';

                try {
                    await window.AIAssistant.generateScript({
                        description:   desc,
                        stepId:        step.id,
                        pipelineId:    ctx.pipeline_id,
                        interfaceId:   ctx.interface_id,
                        messageType:   ctx.message_type,
                        stepType:      step.stepType,
                        currentScript: step.config?.script || ''
                    }, (_tok, full) => {
                        lastScript = full;
                        // Strip outer ```javascript fences for display in dark pre block
                        preview.textContent = full
                            .replace(/^```(?:javascript)?\n?/, '')
                            .replace(/\n?```$/, '');
                    });
                    insertBtn.style.display = '';
                } catch (e) {
                    preview.textContent = '⚠️ ' + e.message;
                } finally {
                    genBtn.disabled = false;
                    genBtn.textContent = 'Generate';
                }
            });

            insertBtn.addEventListener('click', () => {
                if (!lastScript) return;
                // Strip markdown fences — editors expect raw JS
                const raw = lastScript
                    .replace(/^```(?:javascript)?\n?/, '')
                    .replace(/\n?```$/, '')
                    .trim();

                // Try ScriptEnrichmentEditor instance first, then textarea fallback
                const editorInst = window.scriptEnrichmentEditorInstance ||
                    (window.activeScriptEditor);
                if (editorInst && typeof editorInst.setValue === 'function') {
                    editorInst.setValue(raw);
                } else {
                    const ta = document.querySelector(
                        '#scriptEnrichmentEditorContainer textarea, textarea[id*="script"]'
                    );
                    if (ta) {
                        ta.value = raw;
                        ta.dispatchEvent(new Event('input', { bubbles: true }));
                    }
                }

                // Also store in step config so it survives without saving
                if (!step.config) step.config = {};
                step.config.script = raw;

                insertBtn.textContent = '✓ Inserted';
                setTimeout(() => { insertBtn.textContent = 'Insert into editor'; }, 2000);
            });
        }

        // Wire: quick-ask chips
        container.querySelectorAll('.ai-tab-quick').forEach(btn => {
            btn.addEventListener('click', () => {
                const q = `[Step: ${esc(step.stepName || step.stepType)}] ${btn.dataset.q}`;
                if (window.AIAssistant) window.AIAssistant.openWithContext(ctx, q);
            });
        });

        // Wire: open companion button
        const openBtn = container.querySelector('#ai-tab-open-companion');
        if (openBtn) {
            openBtn.addEventListener('click', () => {
                if (window.AIAssistant) window.AIAssistant.openWithContext(ctx,
                    `I'm looking at the "${step.stepName || step.stepType}" step (type: ${step.stepType}). How does it work?`);
            });
        }
    }
}

// Export
if (typeof window !== 'undefined') {
    window.PropertiesPanel = PropertiesPanel;
}

if (typeof module !== 'undefined' && module.exports) {
    module.exports = PropertiesPanel;
}
