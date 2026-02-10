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

        // Get modal elements
        const modal = document.getElementById('stepPropertiesModal');
        const modalTitle = document.getElementById('stepModalTitle');
        const formTabContent = document.getElementById('formTabContent');
        const variablesTabContent = document.getElementById('variablesTabContent');
        const jsonTabContent = document.getElementById('jsonTabContent');
        const docsTabContent = document.getElementById('docsTabContent');

        if (!modal || !formTabContent || !variablesTabContent || !jsonTabContent || !docsTabContent) {
            console.error('Step properties modal or tab containers not found');
            return;
        }

        // Update modal title
        if (modalTitle) {
            const prefix = isPreview ? 'Preview: ' : '';
            modalTitle.textContent = prefix + (step.stepName || 'Step Configuration');
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
    }

    /**
     * Find step's position in pipeline (layer and index)
     */
    findStepPosition(step) {
        const pipeline = this.builder.pipeline;

        console.log('🔍 Finding step position for:', {
            stepId: step.id,
            stepName: step.stepName,
            stepType: step.stepType
        });

        if (!pipeline) {
            console.warn('❌ Pipeline not found');
            return { layerName: null, stepIndex: -1 };
        }

        console.log('📋 Pipeline structure:', {
            hasPipeline: !!pipeline,
            hasLayers: !!pipeline.layers,
            layerKeys: pipeline.layers ? Object.keys(pipeline.layers) : []
        });

        if (!pipeline.layers) {
            console.warn('❌ Pipeline layers not found');
            return { layerName: null, stepIndex: -1 };
        }

        // Search through layers - note: layers have executionGroups, not direct steps
        const layerNames = ['pre', 'core', 'post'];
        for (const layerName of layerNames) {
            const layer = pipeline.layers[layerName];

            console.log(`🔎 Checking ${layerName} layer:`, {
                exists: !!layer,
                hasExecutionGroups: !!layer?.executionGroups,
                groupCount: layer?.executionGroups?.length || 0
            });

            if (layer && layer.executionGroups && layer.executionGroups.length > 0) {
                // Flatten all steps from all execution groups to get sequential index
                const allSteps = [];
                layer.executionGroups.forEach(group => {
                    if (group.steps) {
                        allSteps.push(...group.steps);
                    }
                });

                console.log(`Found ${allSteps.length} total steps in ${layerName} layer`);
                if (allSteps.length > 0) {
                    console.log('Sample step from layer:', allSteps[0]);
                }

                const stepIndex = allSteps.findIndex(s => {
                    const match = s.id === step.id ||                // Match by ID
                                  (s.stepName === step.stepName && s.stepType === step.stepType); // Fallback: name + type

                    if (match) {
                        console.log('✅ Found matching step:', s);
                    }
                    return match;
                });

                if (stepIndex !== -1) {
                    console.log(`✅ Step found in ${layerName} at index ${stepIndex}`);
                    return { layerName, stepIndex };
                }
            }
        }

        console.warn('⚠️ Step not found in any layer');
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
                        // Pass step position for correct variable display
                        layerName: stepPosition.layerName,
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
            ${step.stepType === 'custom' || step.scriptContent ? this.createScriptSection(step) : ''}

            ${actionButtons}
        `;

        // Attach event listeners
        this.attachFormEvents(form, step, isPreview);

        return form;
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
                            placeholder='{
  "stepName": "My Custom Step",
  "stepType": "pre.validation",
  "sequence": 10,
  "enabled": true,
  "config": {
    "key": "value"
  }
}'
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
                            ${docs.parameters.map(p => `
                                <tr>
                                    <td style="padding: 0.75rem; border: 1px solid #e5e7eb; font-family: monospace; color: #059669;">${p.name}</td>
                                    <td style="padding: 0.75rem; border: 1px solid #e5e7eb;"><code style="font-size: 0.85rem;">${p.type}</code></td>
                                    <td style="padding: 0.75rem; border: 1px solid #e5e7eb; text-align: center;">${p.required ? '✓' : '-'}</td>
                                    <td style="padding: 0.75rem; border: 1px solid #e5e7eb; line-height: 1.5;">${p.description}</td>
                                </tr>
                            `).join('')}
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
                        <input type="checkbox" id="stepEnabled" ${step.enabled ? 'checked' : ''}>
                        <span>Enabled</span>
                    </label>
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

        // Reference-based template system:
        // 1. Check if step uses standard template (use_standard_template: true)
        // 2. If so, we'll fetch template mappings asynchronously
        // 3. Custom overrides can be stored in step.config.custom_overrides
        const usesStandardTemplate = step.config?.use_standard_template === true;
        const customOverrides = step.config?.custom_overrides || [];
        const embeddedMappings = step.config?.embedded_mappings;
        let mappings = [];
        let mappingSource = 'none';

        // Priority: embedded_mappings > mappings array > template reference
        if (embeddedMappings) {
            if (Array.isArray(embeddedMappings)) {
                mappings = embeddedMappings;
            } else if (embeddedMappings.atomicMappings && Array.isArray(embeddedMappings.atomicMappings)) {
                mappings = embeddedMappings.atomicMappings;
            } else if (embeddedMappings.mappings && Array.isArray(embeddedMappings.mappings)) {
                mappings = embeddedMappings.mappings;
            } else if (embeddedMappings.custom_mapping_config && Array.isArray(embeddedMappings.custom_mapping_config)) {
                mappings = embeddedMappings.custom_mapping_config;
            }
            mappingSource = 'embedded';
            console.log('🗺️ Using embedded_mappings:', mappings.length, 'mappings');
        } else if (step.config?.mappings && Array.isArray(step.config.mappings) && step.config.mappings.length > 0) {
            mappings = step.config.mappings;
            mappingSource = 'config';
            console.log('🗺️ Using config.mappings:', mappings.length, 'mappings');
        } else if (usesStandardTemplate) {
            // Template reference - will be loaded asynchronously
            mappingSource = 'template_reference';
            console.log('🗺️ Uses standard template - will fetch asynchronously');
        }

        const mappingCount = mappings.length;
        const configJSON = JSON.stringify(step.config, null, 2);

        return `
            <div class="form-section">
                <h4>HL7→FHIR Mapping Configuration</h4>

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
                            ${mappingSource === 'template_reference' ?
                                '<i class="fas fa-spinner fa-spin"></i> Loading standard template...' :
                                `<strong>${mappingCount}</strong> mappings configured`
                            }
                            ${mappingSource === 'embedded' ? '<span style="color: #059669; font-weight: 500;">(from wizard)</span>' : ''}
                            ${mappingSource === 'config' && mappingCount > 0 ? '<span style="color: #3b82f6; font-weight: 500;">(custom)</span>' : ''}
                            ${mappingSource === 'none' ? '<span style="color: #f59e0b; font-weight: 500;">(using standard template at runtime)</span>' : ''}
                        </span>
                        <div style="display: flex; gap: 0.5rem; align-items: center;">
                            <button id="loadStandardTemplateBtn" class="btn btn-secondary" style="font-size: 0.8rem; padding: 0.4rem 0.8rem;" title="Load mappings from the standard template for this message type">
                                <i class="fas fa-download"></i> Load Standard Template
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
                    " data-auto-load-template="${mappingSource === 'template_reference' || mappingSource === 'none' ? 'true' : 'false'}">
                        ${mappingSource === 'template_reference' ?
                            '<div style="padding: 2rem; text-align: center; color: #64748b;"><i class="fas fa-spinner fa-spin fa-2x"></i><p style="margin-top: 1rem;">Loading standard template mappings...</p></div>' :
                            this.renderMappingTable(mappings)
                        }
                    </div>

                    <div style="margin-top: 0.75rem; display: flex; gap: 0.5rem;">
                        <button id="addMappingBtn" class="btn btn-secondary" style="font-size: 0.875rem; padding: 0.5rem 1rem;">
                            <i class="fas fa-plus"></i> Add Mapping
                        </button>
                    </div>
                </div>

                <!-- Tab 2: JSON Editor -->
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
            </div>
        `;
    }

    /**
     * Render mapping table
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

        return `
            <table style="width: 100%; border-collapse: collapse; font-size: 0.875rem;">
                <thead style="background: #f8fafc; position: sticky; top: 0;">
                    <tr>
                        <th style="padding: 0.75rem; text-align: left; border-bottom: 2px solid #e5e7eb; font-weight: 600;">HL7 Field</th>
                        <th style="padding: 0.75rem; text-align: left; border-bottom: 2px solid #e5e7eb; font-weight: 600;">FHIR Path</th>
                        <th style="padding: 0.75rem; text-align: left; border-bottom: 2px solid #e5e7eb; font-weight: 600;">Data Type</th>
                        <th style="padding: 0.75rem; text-align: left; border-bottom: 2px solid #e5e7eb; font-weight: 600; width: 80px;">Actions</th>
                    </tr>
                </thead>
                <tbody>
                    ${mappings.map((mapping, index) => `
                        <tr class="mapping-row" data-index="${index}" style="border-bottom: 1px solid #f1f5f9; cursor: pointer; transition: all 0.2s ease;"
                            onclick="window.propertiesPanel.editMapping(${index})"
                            onmouseover="this.style.background='#f0f9ff'; this.style.transform='translateX(4px)'"
                            onmouseout="this.style.background=''; this.style.transform='translateX(0)'">
                            <td style="padding: 0.75rem;">
                                <code style="background: #dbeafe; padding: 0.25rem 0.5rem; border-radius: 3px; color: #1e3a8a; font-weight: 500;">
                                    ${mapping.hl7Field || mapping.sourceField || mapping.sourcePath || 'N/A'}
                                </code>
                            </td>
                            <td style="padding: 0.75rem;">
                                <code style="background: #fbcfe8; padding: 0.25rem 0.5rem; border-radius: 3px; color: #831843; font-weight: 500;">
                                    ${mapping.fhirPath || mapping.targetField || mapping.targetPath || 'N/A'}
                                </code>
                            </td>
                            <td style="padding: 0.75rem; color: #64748b;">
                                ${mapping.dataType || mapping.hl7DataType || mapping.transformType || '-'}
                            </td>
                            <td style="padding: 0.75rem;">
                                <button class="delete-mapping-btn" data-index="${index}" onclick="event.stopPropagation(); window.propertiesPanel.deleteMapping(${index})" style="
                                    background: none;
                                    border: none;
                                    color: #dc2626;
                                    cursor: pointer;
                                    padding: 0.25rem 0.5rem;
                                    transition: all 0.2s ease;
                                "
                                onmouseover="this.style.color='#991b1b'; this.style.transform='scale(1.1)'"
                                onmouseout="this.style.color='#dc2626'; this.style.transform='scale(1)'"
                                title="Delete">
                                    <i class="fas fa-trash"></i>
                                </button>
                            </td>
                        </tr>
                    `).join('')}
                </tbody>
            </table>
        `;
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
                        Available variables: <code>input</code> (parsed message data)<br>
                        Return modified data or throw error
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
     * Create Try-Catch Configuration UI
     * Uses TryCatchBuilder OOP component
     */
    createTryCatchUI(step) {
        if (!step.config) {
            step.config = {};
        }

        const section = document.createElement('div');
        section.className = 'form-section';
        section.innerHTML = `
            <div id="try-catch-builder-container" style="margin-top: 1rem;"></div>
        `;

        setTimeout(() => {
            const container = document.getElementById('try-catch-builder-container');
            if (container && typeof TryCatchBuilder !== 'undefined') {
                this.tryCatchBuilder = new TryCatchBuilder(container, step.config);
                this.tryCatchBuilder.init();
                console.log('✅ TryCatchBuilder initialized with config:', step.config);

                container.addEventListener('configChange', (e) => {
                    step.config = e.detail.config;
                    console.log('📝 Try-Catch config updated:', step.config);
                });
            } else {
                console.error('❌ TryCatchBuilder not loaded or container not found');
            }
        }, 0);

        return section.outerHTML;
    }

    /**
     * Create Retry Configuration UI
     * Inline form (no separate builder needed - simple config)
     */
    createRetryUI(step) {
        if (!step.config) {
            step.config = {};
        }

        const config = step.config;
        const maxRetries = config.maxRetries || 3;
        const delayMs = config.delayMs || 1000;
        const backoffType = config.backoffType || 'fixed';
        const maxDelayMs = config.maxDelayMs || 30000;
        const childSteps = config.childSteps || [];

        // Get available steps for assignment
        let availableSteps = [];
        try {
            const pipeline = window.pipelineBuilder?.getPipeline();
            const currentStep = window.pipelineBuilder?.currentStep;
            let allSteps = [];
            if (pipeline?.getAllSteps) {
                allSteps = pipeline.getAllSteps();
            } else {
                (pipeline?.executionGroups || []).forEach(g => { if (g.steps) allSteps.push(...g.steps); });
            }
            availableSteps = allSteps.filter(s => currentStep && s.id !== currentStep.id);
        } catch (e) { /* ignore */ }

        const usedIds = new Set(childSteps);
        const available = availableSteps.filter(s => !usedIds.has(s.id));

        const chipsHtml = childSteps.map((id, idx) => {
            const s = availableSteps.find(st => st.id === id);
            const name = s ? (s.stepName || s.step_name || id.substring(0, 12)) : id.substring(0, 12);
            return `<span class="retry-step-chip" style="
                display: inline-flex; align-items: center; gap: 4px;
                padding: 4px 10px; border-radius: 6px; font-size: 12px;
                background: rgba(168, 85, 247, 0.1); border: 1px solid rgba(168, 85, 247, 0.25);
                margin: 2px;">
                <span style="font-weight: 600; color: #a855f7; font-size: 10px;">${idx + 1}.</span>
                ${name}
                <button class="retry-remove-step" data-step-id="${id}" style="
                    background: none; border: none; cursor: pointer; color: var(--text-tertiary);
                    padding: 0 2px; font-size: 14px; line-height: 1;">&times;</button>
            </span>`;
        }).join('') || '<span style="font-size: 12px; color: var(--text-tertiary); font-style: italic;">No steps assigned</span>';

        const section = document.createElement('div');
        section.className = 'form-section';
        section.innerHTML = `
            <div class="retry-builder" style="padding: 0;">
                <h4 style="margin: 0 0 4px; font-size: 14px;">Retry Configuration</h4>
                <p style="margin: 0 0 12px; font-size: 12px; color: var(--text-secondary);">
                    Retries child steps on failure with configurable backoff strategy.
                </p>

                <div class="form-group" style="margin-bottom: 10px;">
                    <label style="font-weight: 600; font-size: 13px;">Max Retries</label>
                    <input type="number" id="retryMaxRetries" class="form-control" value="${maxRetries}" min="1" max="20" style="margin-top: 4px;">
                </div>

                <div class="form-group" style="margin-bottom: 10px;">
                    <label style="font-weight: 600; font-size: 13px;">Backoff Type</label>
                    <select id="retryBackoffType" class="form-control" style="margin-top: 4px;">
                        <option value="fixed" ${backoffType === 'fixed' ? 'selected' : ''}>Fixed - Same delay each time</option>
                        <option value="exponential" ${backoffType === 'exponential' ? 'selected' : ''}>Exponential - 1s, 2s, 4s, 8s...</option>
                        <option value="linear" ${backoffType === 'linear' ? 'selected' : ''}>Linear - 1s, 2s, 3s, 4s...</option>
                    </select>
                </div>

                <div class="form-group" style="margin-bottom: 10px;">
                    <label style="font-weight: 600; font-size: 13px;">Initial Delay (ms)</label>
                    <input type="number" id="retryDelayMs" class="form-control" value="${delayMs}" min="0" step="100" style="margin-top: 4px;">
                </div>

                <div class="form-group" style="margin-bottom: 10px;">
                    <label style="font-weight: 600; font-size: 13px;">Max Delay Cap (ms)</label>
                    <input type="number" id="retryMaxDelayMs" class="form-control" value="${maxDelayMs}" min="0" step="1000" style="margin-top: 4px;">
                    <small style="color: var(--text-tertiary);">Caps delay for exponential/linear backoff</small>
                </div>

                <div style="margin-top: 12px; padding: 10px 14px; border: 1px dashed rgba(168, 85, 247, 0.3); border-radius: 10px; background: rgba(168, 85, 247, 0.04);">
                    <div style="display: flex; align-items: center; gap: 8px; margin-bottom: 6px;">
                        <span style="font-weight: 700; font-size: 11px; text-transform: uppercase; letter-spacing: 0.5px; color: #a855f7;">Child Steps</span>
                        <span style="font-size: 11px; color: var(--text-tertiary);">${childSteps.length} step${childSteps.length !== 1 ? 's' : ''}</span>
                    </div>
                    <p style="margin: 0 0 8px; font-size: 11px; color: var(--text-secondary);">Steps to execute (and retry on failure)</p>
                    <div id="retryChildChips" style="min-height: 28px; margin-bottom: 8px;">${chipsHtml}</div>
                    ${available.length > 0 ? `
                        <div style="display: flex; gap: 6px; align-items: center;">
                            <select id="retryAddStepSelect" class="form-control" style="flex: 1; font-size: 12px; padding: 4px 8px;">
                                <option value="">-- Add step --</option>
                                ${available.map(s => `<option value="${s.id}">${s.stepName || s.step_name || s.id.substring(0, 12)}</option>`).join('')}
                            </select>
                            <button id="retryAddStepBtn" class="btn btn-sm" style="
                                font-size: 12px; padding: 4px 10px; background: #a855f7; color: white;
                                border: none; border-radius: 6px; cursor: pointer;">+ Add</button>
                        </div>
                    ` : ''}
                </div>

                <div style="margin-top: 12px; padding: 10px 14px; background: rgba(168, 85, 247, 0.06); border: 1px solid rgba(168, 85, 247, 0.15); border-radius: 8px; font-size: 12px; color: var(--text-secondary);">
                    <strong style="color: var(--text-primary);">Context Variables:</strong><br>
                    Child steps receive <code>_retry.attempt</code>, <code>_retry.maxRetries</code>, <code>_retry.isRetry</code>
                </div>
            </div>
        `;

        // Attach events after DOM insertion
        setTimeout(() => {
            const updateConfig = () => {
                step.config.maxRetries = parseInt(document.getElementById('retryMaxRetries')?.value) || 3;
                step.config.backoffType = document.getElementById('retryBackoffType')?.value || 'fixed';
                step.config.delayMs = parseInt(document.getElementById('retryDelayMs')?.value) || 1000;
                step.config.maxDelayMs = parseInt(document.getElementById('retryMaxDelayMs')?.value) || 30000;
                console.log('📝 Retry config updated:', step.config);
            };

            ['retryMaxRetries', 'retryBackoffType', 'retryDelayMs', 'retryMaxDelayMs'].forEach(id => {
                const el = document.getElementById(id);
                if (el) el.addEventListener('change', updateConfig);
            });

            // Add step button
            const addBtn = document.getElementById('retryAddStepBtn');
            const addSelect = document.getElementById('retryAddStepSelect');
            if (addBtn && addSelect) {
                addBtn.addEventListener('click', () => {
                    if (!addSelect.value) return;
                    if (!step.config.childSteps) step.config.childSteps = [];
                    if (!step.config.childSteps.includes(addSelect.value)) {
                        step.config.childSteps.push(addSelect.value);
                        console.log('📝 Retry child step added:', addSelect.value);
                        // Re-render the properties panel to show updated chips
                        if (window.pipelineBuilder?.propertiesPanel) {
                            window.pipelineBuilder.propertiesPanel.showStepProperties(step);
                        }
                    }
                });
            }

            // Remove step buttons
            document.querySelectorAll('.retry-remove-step').forEach(btn => {
                btn.addEventListener('click', (e) => {
                    e.stopPropagation();
                    const stepId = btn.dataset.stepId;
                    if (step.config.childSteps) {
                        step.config.childSteps = step.config.childSteps.filter(id => id !== stepId);
                        console.log('📝 Retry child step removed:', stepId);
                        if (window.pipelineBuilder?.propertiesPanel) {
                            window.pipelineBuilder.propertiesPanel.showStepProperties(step);
                        }
                    }
                });
            });
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
                this.connectorConfigBuilder = new ConnectorConfigBuilder(container, step.config, direction);
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

        // Read current config with defaults matching backend (fhir_validation_executor.go line 56-60)
        const validationLevel = (step.config.validation_level || 'standard').toLowerCase();
        const requiredResources = step.config.required_resources || [];
        const validateReferences = step.config.validate_references !== false;
        const validateRequiredFields = step.config.validate_required_fields !== false;
        const failOnError = step.config.fail_on_error === true;

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
            basic: 'Checks only that each resource has a valid <code>resourceType</code> and <code>id</code> field. Fastest execution.',
            standard: 'Basic checks + required field validation per resource type (14 types) + internal bundle reference checking. Recommended for most pipelines.',
            strict: 'Standard checks + full R4 JSON schema validation using FHIR specification schemas (146 resource types). Most thorough but slower.'
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

            <!-- Validate Required Fields -->
            <div class="form-group" id="fhirValidateRequiredFieldsGroup" style="opacity: ${toggleOpacity};">
                <label class="fhir-toggle-label">
                    <input type="checkbox" id="fhirValidateRequiredFields" ${validateRequiredFields ? 'checked' : ''}>
                    <span>Validate Required Fields</span>
                </label>
                <small class="form-text text-muted" style="display: block; margin-top: 4px;">
                    Check that FHIR-spec-required fields are present per resource type (e.g., Encounter must have <code>status</code> and <code>class</code>). Applies at Standard and Strict levels.
                </small>
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
                    <strong>Basic:</strong> Only checks <code>resourceType</code> and <code>id</code> exist.<br>
                    <strong>Standard:</strong> Basic + required field checks (14 resource types) + reference validation.<br>
                    <strong>Strict:</strong> Standard + full R4 JSON schema validation (146 resource schemas).
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

        this.showStepProperties(this.currentStep);
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
        this.showStepProperties(this.currentStep);
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

        this.showStepProperties(this.currentStep);
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

        // Get mapping (undefined for new mapping)
        const mapping = index !== undefined ? this.currentStep.config.mappings[index] : {};

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
                                </select>
                                <button class="btn btn-secondary" style="font-size: 0.85rem; padding: 0.5rem 0.75rem; flex-shrink: 0;" onclick="window.propertiesPanel.browseVariables()">
                                    📚 Browse Variables
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
                            <input type="text" id="editHl7Field" value="${mapping.hl7Field || mapping.sourceField || mapping.sourcePath || ''}"
                                style="width: 100%; padding: 0.5rem; border: 1px solid #cbd5e1; border-radius: 4px; display: none;"
                                placeholder="e.g., PID.5 or [\\"Step Name\\"].enriched_data.fieldName">
                            <small style="color: #64748b; font-size: 0.8rem; margin-top: 0.25rem; display: block;">
                                💡 Tip: Select from dropdown or click "Browse Variables" to use enriched data
                            </small>
                        </div>

                        <!-- Target Field (FHIR Path) -->
                        <div class="form-group">
                            <label style="display: flex; align-items: center; gap: 0.5rem; margin-bottom: 0.5rem;">
                                <span style="font-weight: 600;">FHIR Target Path</span>
                            </label>
                            <select id="editFhirPathDropdown" style="width: 100%; padding: 0.5rem; border: 1px solid #cbd5e1; border-radius: 4px; background: white; margin-bottom: 0.5rem;" onchange="window.propertiesPanel.selectFhirPath()">
                                <option value="">-- Select FHIR Resource Path --</option>
                                <optgroup label="Patient Resource">
                                    <option value="Patient.identifier[0].value">Patient.identifier[0].value - Patient ID</option>
                                    <option value="Patient.name[0].family">Patient.name[0].family - Family Name</option>
                                    <option value="Patient.name[0].given[0]">Patient.name[0].given[0] - Given Name</option>
                                    <option value="Patient.birthDate">Patient.birthDate - Date of Birth</option>
                                    <option value="Patient.gender">Patient.gender - Gender</option>
                                    <option value="Patient.address[0].line[0]">Patient.address[0].line[0] - Address Line</option>
                                    <option value="Patient.address[0].city">Patient.address[0].city - City</option>
                                    <option value="Patient.telecom[0].value">Patient.telecom[0].value - Phone</option>
                                    <option value="Patient.maritalStatus.text">Patient.maritalStatus.text - Marital Status</option>
                                </optgroup>
                                <optgroup label="Encounter Resource">
                                    <option value="Encounter.identifier[0].value">Encounter.identifier[0].value - Visit ID</option>
                                    <option value="Encounter.class.code">Encounter.class.code - Encounter Class</option>
                                    <option value="Encounter.period.start">Encounter.period.start - Admit Date</option>
                                    <option value="Encounter.location[0].location.display">Encounter.location[0].location.display - Location</option>
                                    <option value="Encounter.participant[0].individual.display">Encounter.participant[0].individual.display - Provider</option>
                                </optgroup>
                                <optgroup label="Observation Resource">
                                    <option value="Observation.code.coding[0].code">Observation.code.coding[0].code - Test Code</option>
                                    <option value="Observation.valueQuantity.value">Observation.valueQuantity.value - Numeric Value</option>
                                    <option value="Observation.valueQuantity.unit">Observation.valueQuantity.unit - Unit</option>
                                    <option value="Observation.referenceRange[0].text">Observation.referenceRange[0].text - Reference Range</option>
                                    <option value="Observation.status">Observation.status - Status</option>
                                </optgroup>
                            </select>
                            <input type="text" id="editFhirPath" value="${mapping.fhirPath || mapping.targetField || mapping.targetPath || ''}"
                                style="width: 100%; padding: 0.5rem; border: 1px solid #cbd5e1; border-radius: 4px;"
                                placeholder="e.g., Patient.name[0].family">
                            <small style="color: #64748b; font-size: 0.8rem; margin-top: 0.25rem; display: block;">
                                💡 Select from dropdown or type custom FHIR path
                            </small>
                        </div>

                        <!-- Data Type -->
                        <div class="form-group">
                            <label style="font-weight: 600; margin-bottom: 0.5rem; display: block;">Data Type</label>
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
                            <input type="hidden" id="editDataType" value="${mapping.dataType || mapping.hl7DataType || mapping.transformType || ''}">
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
        // HL7 Field Dropdown - auto-fill text input when selected
        const hl7Dropdown = document.getElementById('editHl7FieldDropdown');
        if (hl7Dropdown) {
            hl7Dropdown.addEventListener('change', (e) => {
                if (e.target.value) {
                    document.getElementById('editHl7Field').value = e.target.value;
                }
            });
        }

        // Data Type Dropdown - pre-select if existing value
        const dataTypeDropdown = document.getElementById('editDataTypeDropdown');
        const existingDataType = document.getElementById('editDataType').value;
        if (dataTypeDropdown && existingDataType) {
            dataTypeDropdown.value = existingDataType;
        }
    }

    /**
     * Toggle source input based on source type selection (NO-CODE)
     */
    toggleSourceInput() {
        const sourceType = document.getElementById('editSourceType').value;
        const hl7Dropdown = document.getElementById('editHl7FieldDropdown');
        const customInput = document.getElementById('editHl7Field');

        if (sourceType === 'hl7') {
            // Show HL7 dropdown, hide custom input
            hl7Dropdown.style.display = 'block';
            customInput.style.display = 'none';
        } else if (sourceType === 'enriched') {
            // Hide dropdown, show text input with placeholder for enriched data
            hl7Dropdown.style.display = 'none';
            customInput.style.display = 'block';
            customInput.placeholder = 'e.g., ["database_enrichment"].enriched_data.fieldName';
        } else {
            // Custom XPath - show text input with generic placeholder
            hl7Dropdown.style.display = 'none';
            customInput.style.display = 'block';
            customInput.placeholder = 'Enter custom XPath expression';
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
    selectFhirPath() {
        const dropdown = document.getElementById('editFhirPathDropdown');
        const input = document.getElementById('editFhirPath');
        if (dropdown && input && dropdown.value) {
            input.value = dropdown.value;
        }
    }

    /**
     * Save edited mapping
     */
    saveEditedMapping(index) {
        // Get value from text input (which is populated by dropdowns or manual entry)
        let hl7Field = document.getElementById('editHl7Field').value.trim();
        const fhirPath = document.getElementById('editFhirPath').value.trim();
        const dataType = document.getElementById('editDataType').value.trim();

        // If hl7Field is empty, check if user selected from dropdown but didn't trigger change
        if (!hl7Field) {
            const dropdown = document.getElementById('editHl7FieldDropdown');
            if (dropdown && dropdown.value) {
                hl7Field = dropdown.value;
            }
        }

        if (!hl7Field || !fhirPath) {
            this.builder.dragDropManager.showNotification('HL7 Field and FHIR Path are required', 'error');
            return;
        }

        // Create mapping object
        const mappingObject = {
            hl7Field: hl7Field,
            sourcePath: hl7Field,
            fhirPath: fhirPath,
            targetPath: fhirPath,
            dataType: dataType,
            transformType: dataType
        };

        // Add or update mapping
        if (index === undefined) {
            // Adding new mapping
            this.currentStep.config.mappings.push(mappingObject);
            this.builder.dragDropManager.showNotification('Mapping added', 'success');
        } else {
            // Updating existing mapping
            this.currentStep.config.mappings[index] = mappingObject;
            this.builder.dragDropManager.showNotification('Mapping updated', 'success');
        }

        // Close modal
        document.getElementById('editMappingModal').remove();

        // Auto-save immediately
        this.builder.updateStep(this.currentStep);
        this.builder.savePipeline().then(() => {
            console.log('[Field Mapping] Pipeline auto-saved after mapping change');
        }).catch(err => {
            console.error('[Field Mapping] Auto-save failed:', err);
        });

        // Refresh properties panel
        this.showStepProperties(this.currentStep);

        // Mark as unsaved
        this.builder.markAsUnsaved();
    }

    /**
     * Delete a mapping
     */
    async deleteMapping(index) {
        if (!this.currentStep || !this.currentStep.config || !this.currentStep.config.mappings) {
            return;
        }

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

        // Remove mapping
        this.currentStep.config.mappings.splice(index, 1);

        // Auto-save immediately
        this.builder.updateStep(this.currentStep);
        this.builder.savePipeline().then(() => {
            console.log('[Field Mapping] Pipeline auto-saved after mapping deletion');
        }).catch(err => {
            console.error('[Field Mapping] Auto-save failed:', err);
        });

        // Refresh properties panel
        this.showStepProperties(this.currentStep);

        // Mark as unsaved
        this.builder.markAsUnsaved();

        this.builder.dragDropManager.showNotification('Mapping deleted', 'success');
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
     * Load standard HL7-FHIR template mappings from the database
     */
    async loadStandardTemplateMappings(step) {
        try {
            // Get the message type from the pipeline
            const messageType = this.builder.pipeline?.messageType || 'ADT^A01';

            this.builder.dragDropManager.showNotification(`Loading standard template for ${messageType}...`, 'info');

            // Fetch the standard template mappings from the API
            const response = await window.pipelineAPI.getStandardTemplateMappings(messageType);

            if (!response.success) {
                throw new Error(response.error || 'Failed to fetch template mappings');
            }

            const { template, mappings, mappingCount } = response.data;

            if (!mappings || mappings.length === 0) {
                this.builder.dragDropManager.showNotification(`No standard template found for ${messageType}`, 'warning');
                return;
            }

            // Initialize step config if needed
            if (!step.config) {
                step.config = {};
            }

            // Store the mappings in the step config
            step.config.mappings = mappings;
            step.config.use_template = true;
            step.config.template_id = template?.id;
            step.config.template_name = template?.name;

            console.log(`📚 Loaded ${mappingCount} mappings from template "${template?.name}"`, mappings);

            // Update the mapping table in-place (avoid full modal re-render which loses state)
            const tableContainer = document.getElementById('mappingTableContainer');
            if (tableContainer) {
                tableContainer.innerHTML = this.renderMappingTable(mappings);
                console.log('✅ Mapping table updated in-place');
            }

            // Update the status bar
            const statusText = document.getElementById('mappingStatusText');
            if (statusText) {
                statusText.innerHTML = `<strong>${mappingCount}</strong> mappings configured <span style="color: #059669; font-weight: 500;">(from template: ${template?.name || 'standard'})</span>`;
            }

            this.builder.dragDropManager.showNotification(`Loaded ${mappingCount} mappings from standard template`, 'success');

            // Mark as unsaved
            this.builder.markAsUnsaved();

        } catch (error) {
            console.error('Error loading standard template mappings:', error);
            this.builder.dragDropManager.showNotification(`Failed to load template: ${error.message}`, 'error');
        }
    }

    /**
     * Auto-load template mappings for display only (doesn't modify step config)
     * Called when opening an HL7-FHIR step that uses standard template
     */
    async autoLoadTemplateMappings(step, container) {
        try {
            const messageType = this.builder.pipeline?.messageType || 'ADT^A01';
            console.log(`🔄 Auto-fetching template for ${messageType}...`);

            const response = await window.pipelineAPI.getStandardTemplateMappings(messageType);

            if (!response.success) {
                throw new Error(response.error || 'Failed to fetch template');
            }

            const { template, mappings, mappingCount } = response.data;

            if (!mappings || mappings.length === 0) {
                container.innerHTML = `
                    <div style="padding: 2rem; text-align: center; color: #64748b;">
                        <i class="fas fa-info-circle fa-2x" style="color: #f59e0b;"></i>
                        <p style="margin-top: 1rem;">No standard template found for ${messageType}</p>
                        <p style="font-size: 0.85rem;">Click "Load Standard Template" to import one, or add custom mappings below.</p>
                    </div>
                `;
                return;
            }

            // Update the status bar
            const statusText = document.getElementById('mappingStatusText');
            if (statusText) {
                statusText.innerHTML = `
                    <strong>${mappingCount}</strong> mappings
                    <span style="color: #059669; font-weight: 500;">(standard template: ${template?.name || messageType})</span>
                `;
            }

            // Render the mappings table
            container.innerHTML = this.renderMappingTable(mappings);

            // Store template reference for display purposes (not modifying step config)
            this._displayedTemplateMappings = mappings;
            this._displayedTemplateName = template?.name;

            console.log(`✅ Auto-loaded ${mappingCount} mappings from template "${template?.name}"`);

        } catch (error) {
            console.error('Error auto-loading template mappings:', error);
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
            stepType: step.stepType,
            layer: step.layer
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

            // Validate required fields toggle
            const reqFieldsCheckbox = form.querySelector('#fhirValidateRequiredFields');
            if (reqFieldsCheckbox) {
                step.config.validate_required_fields = reqFieldsCheckbox.checked;
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

            // Remove dead fhir_version key from old config
            delete step.config.fhir_version;

            console.log('[PropertiesPanel] ✅ Saved FHIR Validation config:', step.config);
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
                resultDiv.innerHTML = `<span style="color: #ef4444;">❌ ${errorMsg}</span>`;
                scriptContent.style.borderColor = '#ef4444';
                this.builder.dragDropManager.showNotification('Script validation failed', 'error');

                // Show detailed errors if available
                if (result.details) {
                    console.error('Script validation details:', result.details);
                }
            }
        } catch (error) {
            console.error('Script validation error:', error);
            resultDiv.innerHTML = `<span style="color: #ef4444;">❌ Validation failed: ${error.message}</span>`;
            this.builder.dragDropManager.showNotification('Failed to validate script', 'error');
        } finally {
            validateBtn.disabled = false;
            validateBtn.innerHTML = '🔍 Validate Script';
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

        // Special handling for Try-Catch container step
        if (VisualStep.isTryCatchStep(step)) {
            console.log('🛡️ Using TryCatch Builder for try-catch container');
            return this.createTryCatchUI(step);
        }

        // Special handling for Retry container step
        if (VisualStep.isRetryStep(step)) {
            console.log('🔄 Using Retry Builder for retry container');
            return this.createRetryUI(step);
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

                case 'checkbox':
                    const checked = value === true || value === 'true' ? 'checked' : '';
                    html += `<label class="checkbox-label"><input type="checkbox" name="config_${field.key}" ${checked}> ${field.checkboxLabel || field.label}</label>`;
                    break;

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
            }
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
                description: 'Transforms HL7 v2 messages to FHIR format using configured mapping rules. This is the core transformation step that converts healthcare data from legacy HL7 format to modern FHIR resources. You can use data from previous enrichment steps (database lookups, API calls, scripts) in your field mappings.',
                useCases: [
                    'Convert ADT^A01 (patient admission) to FHIR Patient + Encounter',
                    'Transform ORU^R01 (lab results) to FHIR Observation + DiagnosticReport',
                    'Map ORM^O01 (order) to FHIR ServiceRequest',
                    'Convert DFT^P03 (billing) to FHIR Claim',
                    'Enrich FHIR resources with data from previous enrichment steps',
                    'Apply organization-specific mapping customizations with dynamic data'
                ],
                example: {
                    fhir_version: 'R4',
                    use_template: true,
                    mappings: [
                        {
                            comment: 'Basic HL7 field mapping',
                            hl7Field: 'PID.5',
                            fhirPath: 'Patient.name[0].family',
                            dataType: 'string'
                        },
                        {
                            comment: 'Use enriched data from database step',
                            hl7Field: '["database_enrichment"].enriched_data.insuranceProvider',
                            fhirPath: 'Coverage.payor[0].display',
                            dataType: 'string'
                        },
                        {
                            comment: 'Use enriched data from script step',
                            hl7Field: '["Script_Enrichment"].enriched_data.riskScore',
                            fhirPath: 'Observation.valueQuantity.value',
                            dataType: 'decimal'
                        },
                        {
                            comment: 'Use enriched data from API call',
                            hl7Field: '["API_Enrichment"].enriched_data.externalPatientId',
                            fhirPath: 'Patient.identifier[1].value',
                            dataType: 'string'
                        }
                    ],
                    resource_mapping: {
                        'Patient': 'PID segment + enriched data',
                        'Encounter': 'PV1 segment',
                        'Observation': 'OBX segments + calculated risk scores'
                    }
                },
                parameters: [
                    { name: 'fhir_version', type: 'string', required: true, description: 'Target FHIR version (R4, R5, STU3)' },
                    { name: 'use_template', type: 'boolean', required: false, description: 'Use wizard-configured mappings (default: true)' },
                    { name: 'mappings', type: 'array', required: false, description: 'Array of field mappings. Each mapping can reference HL7 fields or enriched data from previous steps using ["step_name"].enriched_data.fieldName format' }
                ],
                referenceVariables: {
                    title: 'Using Enriched Data in Field Mappings',
                    description: 'You can reference data from previous enrichment steps in your field mappings. Click the "Variables" tab to see all available variables from previous steps.',
                    examples: [
                        {
                            scenario: 'Database Enrichment Result',
                            hl7Field: '["database_enrichment"].enriched_data.chronicConditions',
                            fhirPath: 'Patient.extension[0].valueInteger',
                            explanation: 'Maps the chronicConditions field from database enrichment to a FHIR Patient extension'
                        },
                        {
                            scenario: 'Script Calculation Result',
                            hl7Field: '["Script_Enrichment"].enriched_data.riskLevel',
                            fhirPath: 'RiskAssessment.prediction[0].qualitativeRisk.text',
                            explanation: 'Uses calculated risk level from script enrichment in FHIR RiskAssessment resource'
                        },
                        {
                            scenario: 'API Response Data',
                            hl7Field: '["API_Enrichment"].enriched_data.externalSystemId',
                            fhirPath: 'Patient.identifier[1].value',
                            explanation: 'Adds external system ID from API call as additional patient identifier'
                        },
                        {
                            scenario: 'Metadata Field',
                            hl7Field: 'metadata.customField',
                            fhirPath: 'MessageHeader.extension[0].valueString',
                            explanation: 'Includes custom metadata field added in metadata enrichment step'
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
            'control.try_catch': {
                description: 'Container step that wraps child steps in error handling with try/catch/finally blocks. If a step in the Try block fails, execution moves to the Catch block. The Finally block always executes regardless of success or failure.',
                useCases: [
                    'Wrap risky API enrichment calls - catch errors and use fallback data',
                    'Handle database connection failures gracefully during enrichment',
                    'Ensure cleanup/audit logging always runs in Finally block',
                    'Suppress non-critical transformation errors to avoid pipeline failure',
                    'Log errors for monitoring while allowing pipeline to continue'
                ],
                example: {
                    trySteps: ['api-enrichment-step', 'transform-result'],
                    catchSteps: ['log-error', 'use-fallback-data'],
                    finallySteps: ['audit-log'],
                    onError: 'catch'
                },
                parameters: [
                    { name: 'trySteps', type: 'Array<string>', required: true, description: 'Step IDs to execute in the try block. Execution stops at first failure.' },
                    { name: 'catchSteps', type: 'Array<string>', required: false, description: 'Step IDs to execute when a try step fails. Receives _error context.' },
                    { name: 'finallySteps', type: 'Array<string>', required: false, description: 'Step IDs that always execute (success or failure). Receives _trySuccess context.' },
                    { name: 'onError', type: 'enum', required: false, description: '"catch" (default) - run catch steps and continue; "suppress" - ignore error, continue; "rethrow" - propagate error, stop pipeline' }
                ]
            },
            'control.retry': {
                description: 'Container step that retries child steps on failure with configurable backoff strategy. Useful for transient errors like network timeouts or temporary service unavailability.',
                useCases: [
                    'Retry failed API calls with exponential backoff',
                    'Handle transient database connection errors',
                    'Retry file operations that may fail due to locks',
                    'Resilient external service integration'
                ],
                example: {
                    childSteps: ['call-external-api', 'process-response'],
                    maxRetries: 3,
                    delayMs: 1000,
                    backoffType: 'exponential',
                    maxDelayMs: 30000
                },
                parameters: [
                    { name: 'childSteps', type: 'Array<string>', required: true, description: 'Step IDs to execute (and retry on failure).' },
                    { name: 'maxRetries', type: 'number', required: false, description: 'Maximum retry attempts. Default: 3' },
                    { name: 'delayMs', type: 'number', required: false, description: 'Initial delay between retries in milliseconds. Default: 1000' },
                    { name: 'backoffType', type: 'enum', required: false, description: '"fixed" (same delay), "exponential" (doubles each time), "linear" (increases linearly)' },
                    { name: 'maxDelayMs', type: 'number', required: false, description: 'Maximum delay cap for exponential/linear backoff. Default: 30000' }
                ]
            }
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
            description: 'Fetches data from external systems via configurable inbound connectors. Supports TCP/MLLP, HTTP/REST, file listeners, databases, message queues, and cloud storage.',
            useCases: [
                'Fetch patient data from an external database mid-pipeline',
                'Read configuration from a file or cloud storage',
                'Poll a message queue for additional data',
                'Query a REST API for supplemental information'
            ],
            example: { connectorType: 'postgresql_inbound', config: { host: 'db-server', port: 5432, database: 'ehr' }, outputField: 'enriched.external_data', timeoutMs: 30000 },
            parameters: [
                { name: 'connectorType', type: 'string', required: true, description: 'The type of inbound connector (e.g., tcp_mllp_inbound, http_rest_inbound, postgresql_inbound)' },
                { name: 'config', type: 'object', required: true, description: 'Connector-specific configuration (host, port, credentials, etc.) - fields are driven by the connector type config_schema' },
                { name: 'outputField', type: 'string', required: false, description: 'Where to store fetched data in the pipeline (default: enriched.connector_result)' },
                { name: 'timeoutMs', type: 'number', required: false, description: 'Maximum wait time for data fetch in milliseconds (default: 30000)' }
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

        // Add aliases for renamed step types (layer prefix removed)
        docs['enrichment.script'] = docs['pre.enrichment.script'];
        docs['enrichment.api'] = docs['pre.enrichment.api'];
        docs['enrichment.database'] = docs['pre.enrichment.database'];
        docs['field_validation'] = docs['pre.validation'];
        docs['hl7_fhir_transform'] = docs['core.mapping'];
        docs['field_mapping'] = docs['core.transformation'];
        docs['fhir_validation'] = docs['post.validation'];

        // Default documentation for unknown step types
        return docs[stepType] || {
            description: `Configuration for ${stepType} step. This is a custom step type.`,
            useCases: ['Custom transformation logic', 'Specialized data processing'],
            example: { config: 'Custom configuration' },
            parameters: []
        };
    }
}

// Export
if (typeof window !== 'undefined') {
    window.PropertiesPanel = PropertiesPanel;
}

if (typeof module !== 'undefined' && module.exports) {
    module.exports = PropertiesPanel;
}
