/**
 * Properties Panel Manager
 * Manages the right panel for step configuration
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
        const jsonTabContent = document.getElementById('jsonTabContent');
        const docsTabContent = document.getElementById('docsTabContent');

        if (!modal || !formTabContent || !jsonTabContent || !docsTabContent) {
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

        // Different button layout for preview vs edit mode
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
                    <pre style="background: #f3f4f6; padding: 1rem; border-radius: 6px; overflow-x: auto; font-size: 0.875rem;"><code>${JSON.stringify(docs.example, null, 2)}</code></pre>
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
                        <label>Sequence Order</label>
                        <input type="number" id="stepSequence" value="${step.sequence}" min="1">
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
        // Special handling for HL7→FHIR mapping steps
        if (step.stepType === 'core.mapping' || step.templateId === 'hl7-fhir-mapping') {
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
     */
    createMappingConfigSection(step) {
        const mappings = step.config?.mappings || [];
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
                    <div style="margin-bottom: 1rem; padding: 0.75rem; background: #f8fafc; border-radius: 6px; display: flex; justify-content: space-between; align-items: center;">
                        <span style="color: #475569; font-size: 0.875rem;">
                            <strong>${mappingCount}</strong> mappings configured
                            ${step.config?.source === 'wizard' ? '(from wizard)' : ''}
                        </span>
                        <input type="text" id="mappingSearchInput" placeholder="Search mappings..." style="
                            padding: 0.375rem 0.75rem;
                            border: 1px solid #cbd5e1;
                            border-radius: 4px;
                            font-size: 0.875rem;
                            width: 200px;
                        ">
                    </div>

                    <div id="mappingTableContainer" style="
                        max-height: 400px;
                        overflow-y: auto;
                        border: 1px solid #e5e7eb;
                        border-radius: 6px;
                    ">
                        ${this.renderMappingTable(mappings)}
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
                    <small style="color: #64748b;">
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

            // Render the tester
            tester.render(getCurrentStepConfig());

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
            clearBtn.addEventListener('click', () => {
                const textarea = container.querySelector('#jsonConfigInput');
                if (confirm('Are you sure you want to clear the JSON configuration?')) {
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
     * Edit a mapping (single-click edit)
     */
    editMapping(index) {
        if (!this.currentStep || !this.currentStep.config || !this.currentStep.config.mappings) {
            return;
        }

        const mapping = this.currentStep.config.mappings[index];
        if (!mapping) {
            this.builder.dragDropManager.showNotification('Mapping not found', 'error');
            return;
        }

        // Create edit modal
        const editModalHTML = `
            <div id="editMappingModal" class="modal" style="display: flex;">
                <div class="modal-content" style="max-width: 600px;">
                    <div class="modal-header" style="background: linear-gradient(135deg, #1e3a8a 0%, #2563eb 100%); color: white;">
                        <h3><i class="fas fa-edit"></i> Edit Mapping</h3>
                        <button class="modal-close" onclick="document.getElementById('editMappingModal').remove()">&times;</button>
                    </div>
                    <div class="modal-body">
                        <div class="form-group">
                            <label>HL7 Field</label>
                            <input type="text" id="editHl7Field" value="${mapping.hl7Field || mapping.sourceField || mapping.sourcePath || ''}" style="width: 100%; padding: 0.5rem; border: 1px solid #cbd5e1; border-radius: 4px;">
                        </div>
                        <div class="form-group">
                            <label>FHIR Path</label>
                            <input type="text" id="editFhirPath" value="${mapping.fhirPath || mapping.targetField || mapping.targetPath || ''}" style="width: 100%; padding: 0.5rem; border: 1px solid #cbd5e1; border-radius: 4px;">
                        </div>
                        <div class="form-group">
                            <label>Data Type</label>
                            <input type="text" id="editDataType" value="${mapping.dataType || mapping.hl7DataType || mapping.transformType || ''}" style="width: 100%; padding: 0.5rem; border: 1px solid #cbd5e1; border-radius: 4px;">
                        </div>
                    </div>
                    <div class="modal-footer">
                        <button class="btn btn-secondary" onclick="document.getElementById('editMappingModal').remove()">Cancel</button>
                        <button class="btn btn-primary" onclick="window.propertiesPanel.saveEditedMapping(${index})">
                            <i class="fas fa-save"></i> Save Changes
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
    }

    /**
     * Save edited mapping
     */
    saveEditedMapping(index) {
        const hl7Field = document.getElementById('editHl7Field').value;
        const fhirPath = document.getElementById('editFhirPath').value;
        const dataType = document.getElementById('editDataType').value;

        if (!hl7Field || !fhirPath) {
            this.builder.dragDropManager.showNotification('HL7 Field and FHIR Path are required', 'error');
            return;
        }

        // Update mapping
        this.currentStep.config.mappings[index] = {
            hl7Field: hl7Field,
            sourcePath: hl7Field,
            fhirPath: fhirPath,
            targetPath: fhirPath,
            dataType: dataType,
            transformType: dataType
        };

        // Close modal
        document.getElementById('editMappingModal').remove();

        // Refresh properties panel
        this.showStepProperties(this.currentStep);

        // Mark as unsaved
        this.builder.markAsUnsaved();

        this.builder.dragDropManager.showNotification('Mapping updated', 'success');
    }

    /**
     * Delete a mapping
     */
    deleteMapping(index) {
        if (!this.currentStep || !this.currentStep.config || !this.currentStep.config.mappings) {
            return;
        }

        if (!confirm('Are you sure you want to delete this mapping?')) {
            return;
        }

        // Remove mapping
        this.currentStep.config.mappings.splice(index, 1);

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
     * Save step properties
     */
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
     * Close step properties modal
     */
    closeModal() {
        const modal = document.getElementById('stepPropertiesModal');
        if (modal) {
            modal.style.display = 'none';
        }
        this.currentStep = null;
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

        // Configuration textarea (legacy)
        const configText = form.querySelector('#stepConfig')?.value;
        if (configText) {
            try {
                step.config = JSON.parse(configText);
            } catch (error) {
                throw new Error('Invalid JSON in configuration');
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
            'pre.enrichment.metadata': {
                fields: [
                    {
                        key: 'addTimestamp',
                        label: 'Add Timestamp',
                        type: 'checkbox',
                        default: true,
                        checkboxLabel: 'Add receivedAt and processedAt timestamps',
                        help: 'Automatically adds timestamp metadata to messages'
                    },
                    {
                        key: 'addCorrelationId',
                        label: 'Add Correlation ID',
                        type: 'checkbox',
                        default: true,
                        checkboxLabel: 'Generate unique correlation ID (UUID)',
                        help: 'Adds a unique correlation ID for message tracking'
                    },
                    {
                        key: 'addInterfaceId',
                        label: 'Add Interface ID',
                        type: 'checkbox',
                        default: false,
                        checkboxLabel: 'Include interface ID in metadata',
                        help: 'Adds the current interface ID to message metadata'
                    },
                    {
                        key: 'addMessageId',
                        label: 'Add/Extract Message ID',
                        type: 'checkbox',
                        default: false,
                        checkboxLabel: 'Extract or generate message ID',
                        help: 'Extracts message ID from HL7 MSH.10 or generates a new one'
                    },
                    {
                        key: 'customMetadata',
                        label: 'Custom Metadata',
                        type: 'metadata-builder',
                        required: false,
                        help: 'Add custom key-value pairs (e.g., environment, processingNode, facility). Non-technical users can use the form below, technical users can import JSON.'
                    }
                ]
            },
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
                        placeholder: 'database_name',
                        help: 'Database name to connect to'
                    },
                    {
                        key: 'dbUser',
                        label: 'Username',
                        type: 'text',
                        required: true,
                        default: 'ezhealth_user',
                        placeholder: 'username',
                        help: 'Database username'
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
                        key: 'redisKey',
                        label: 'Redis Key Pattern',
                        type: 'text',
                        required: false,
                        placeholder: 'patient:{PID.3}',
                        help: 'Redis key pattern with field placeholders. Example: patient:{PID.3}',
                        showIf: { field: 'databaseType', values: ['redis'] }
                    },
                    {
                        key: 'redisCommand',
                        label: 'Redis Command',
                        type: 'select',
                        required: false,
                        options: [
                            { value: 'GET', label: 'GET - Get string value' },
                            { value: 'HGETALL', label: 'HGETALL - Get all hash fields' },
                            { value: 'SMEMBERS', label: 'SMEMBERS - Get all set members' },
                            { value: 'LRANGE', label: 'LRANGE - Get list range' }
                        ],
                        default: 'GET',
                        help: 'Redis command to execute (only for Redis)',
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
                        rows: 12,
                        placeholder: `// Extract patient date of birth
var dob = getNestedValue(input, "enhancedSegments.PID.fields.7.value");

// Calculate age
var age = calculateAge(dob);

// Return enrichment data
return {
    age: age,
    ageGroup: age < 18 ? "pediatric" : "adult"
};`,
                        help: 'JavaScript code to execute. Use "input" variable for message data. Available functions: getNestedValue(), calculateAge(), parseHL7Date(), console.log()'
                    },
                    {
                        key: 'context',
                        label: 'Context Variables (JSON)',
                        type: 'textarea',
                        rows: 3,
                        placeholder: '{"hospitalId": "HOSPITAL_001", "environment": "production"}',
                        help: 'Additional variables to make available in script context (JSON format)'
                    },
                    {
                        key: 'targetPath',
                        label: 'Target Path',
                        type: 'text',
                        default: 'enriched.script',
                        placeholder: 'enriched.script',
                        help: 'Where to store script result in message data'
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
                fields: [
                    {
                        key: 'fhir_version',
                        label: 'FHIR Version',
                        type: 'select',
                        required: true,
                        options: [
                            { value: 'R4', label: 'FHIR R4' },
                            { value: 'R5', label: 'FHIR R5' }
                        ],
                        default: 'R4'
                    },
                    {
                        key: 'validation_level',
                        label: 'Validation Level',
                        type: 'select',
                        required: true,
                        options: [
                            { value: 'BASIC', label: 'Basic - Structure only' },
                            { value: 'STANDARD', label: 'Standard - Structure + required fields' },
                            { value: 'STRICT', label: 'Strict - Full spec compliance' }
                        ],
                        default: 'STANDARD'
                    }
                ]
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
            'pre.enrichment.metadata': {
                description: 'Adds processing metadata, timestamps, correlation IDs, and custom organizational fields to messages. Enriches messages with tracking, auditing, and contextual information.',
                useCases: [
                    'Add timestamps for message received and processed times (ISO 8601 format)',
                    'Generate unique correlation IDs for message tracking across systems',
                    'Add custom organizational metadata (environment, processing node, version)',
                    'Include interface ID and message ID for troubleshooting',
                    'Tag messages with source system, facility, or department information',
                    'Add processing context (server hostname, region, data center)'
                ],
                example: {
                    addTimestamp: true,
                    addCorrelationId: true,
                    addInterfaceId: false,
                    addMessageId: false,
                    customMetadata: {
                        "processingNode": "server-01",
                        "environment": "production",
                        "facility": "MAIN_CAMPUS",
                        "department": "RADIOLOGY",
                        "version": "2.1.0",
                        "region": "us-east-1"
                    }
                },
                parameters: [
                    { name: 'addTimestamp', type: 'boolean', required: false, description: 'Add receivedAt and processedAt timestamps in ISO 8601 format (e.g., 2025-10-26T14:30:00Z)' },
                    { name: 'addCorrelationId', type: 'boolean', required: false, description: 'Generate and add a unique UUID correlation ID for end-to-end message tracking' },
                    { name: 'addInterfaceId', type: 'boolean', required: false, description: 'Include the interface ID from which the message was received' },
                    { name: 'addMessageId', type: 'boolean', required: false, description: 'Extract or generate a unique message ID (from MSH.10 if available in HL7 messages)' },
                    { name: 'customMetadata', type: 'JSON Object', required: false, description: 'Custom key-value pairs to add as metadata. Must be valid JSON format. Example: {"environment": "prod", "version": "1.0", "facility": "MAIN"}. All values are stored as strings.' }
                ]
            },
            'core.mapping': {
                description: 'Transforms HL7 v2 messages to FHIR format using configured mapping rules. This is the core transformation step that converts healthcare data from legacy HL7 format to modern FHIR resources.',
                useCases: [
                    'Convert ADT^A01 (patient admission) to FHIR Patient + Encounter',
                    'Transform ORU^R01 (lab results) to FHIR Observation + DiagnosticReport',
                    'Map ORM^O01 (order) to FHIR ServiceRequest',
                    'Convert DFT^P03 (billing) to FHIR Claim',
                    'Apply organization-specific mapping customizations'
                ],
                example: {
                    fhir_version: 'R4',
                    use_template: true,
                    mappings: 'Loaded from wizard configuration',
                    resource_mapping: {
                        'Patient': 'PID segment',
                        'Encounter': 'PV1 segment',
                        'Observation': 'OBX segments'
                    }
                },
                parameters: [
                    { name: 'fhir_version', type: 'string', required: true, description: 'Target FHIR version (R4, R5, STU3)' },
                    { name: 'use_template', type: 'boolean', required: false, description: 'Use wizard-configured mappings (default: true)' }
                ]
            },
            'post.validation': {
                description: 'Validates FHIR resources after transformation to ensure compliance with FHIR specification. Catches transformation errors before delivery.',
                useCases: [
                    'Verify FHIR resource structure matches spec',
                    'Validate required fields are present in FHIR resources',
                    'Check cardinality constraints (min/max occurrences)',
                    'Validate data types and code systems',
                    'Ensure references between resources are valid'
                ],
                example: {
                    fhir_version: 'R4',
                    validation_level: 'STANDARD',
                    resource_rules: {
                        validate_structure: true,
                        validate_cardinality: true,
                        validate_data_types: true,
                        validate_references: true
                    },
                    required_resources: {
                        'Patient': { min: 1, max: 1 }
                    }
                },
                parameters: [
                    { name: 'fhir_version', type: 'string', required: true, description: 'FHIR version to validate against' },
                    { name: 'validation_level', type: 'string', required: true, description: 'BASIC, STANDARD, or STRICT validation' }
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
