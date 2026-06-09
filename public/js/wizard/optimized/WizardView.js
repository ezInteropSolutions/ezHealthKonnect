/**
 * WizardView.js - View Layer for Optimized Interface Wizard
 * Implements MVC View pattern with modern UX design
 *
 * Features:
 * - Responsive, accessible UI following our design system
 * - Progressive enhancement with OOB templates
 * - Real-time validation feedback
 * - Smooth animations and transitions
 * - Integration with our existing CSS framework
 */

class WizardView extends EventTarget {
    constructor(containerId = 'wizard-modal-container') {
        super();
        this.containerId = containerId;
        this.container = null;
        this.modal = null;
        this.currentStepElement = null;

        // Animation configuration
        this.animationDuration = 300;
        this.isAnimating = false;

        // Template cache for performance
        this.templateCache = new Map();

        // Duplicate name validation (checked on blur, not on every keystroke)
        this.lastCheckedName = null;
        this.isDuplicateName = false;

        // OOB: Form binding engine for schema-based form handling
        this.formBindingEngine = null;

        this.initializeView();
    }

    /**
     * Initialize form binding engine after container is ready
     * OOB Pattern: Uses FormFieldSchema as single source of truth
     */
    initializeFormBinding() {
        if (typeof FormFieldSchema !== 'undefined' && typeof FormBindingEngine !== 'undefined') {
            this.formBindingEngine = new FormBindingEngine(this.container, FormFieldSchema);
            console.log('✅ FormBindingEngine initialized with schema');
        } else {
            console.warn('⚠️ FormFieldSchema or FormBindingEngine not loaded, using legacy sync');
        }
    }

    /**
     * Initialize the wizard view structure
     */
    initializeView() {
        this.container = document.getElementById(this.containerId);
        if (!this.container) {
            console.error(`Container ${this.containerId} not found`);
            return;
        }

        this.render();
        this.attachEventListeners();
        this.initializeFormBinding(); // OOB: Initialize schema-based form binding
    }

    /**
     * Main render method - creates the complete wizard structure
     */
    render() {
        this.container.innerHTML = this.getModalTemplate();
        this.modal = this.container.querySelector('.wizard-modal-overlay');
        this.setupAccessibility();
    }

    /**
     * Modal template following our design system
     */
    getModalTemplate() {
        return `
            <div class="wizard-modal-overlay" id="wizardModalOverlay" role="dialog" aria-labelledby="wizard-title" aria-modal="true">
                <div class="wizard-modal-container" id="wizardModalContainer">
                    <!-- Top Right Navigation Buttons (Floating) -->
                    <div class="wizard-floating-nav">
                        <button class="wizard-nav-btn" id="wizardPrevious" disabled>
                            <span>←</span> Previous
                        </button>
                        <button class="wizard-nav-btn primary" id="wizardNext">
                            Next <span>→</span>
                        </button>
                        <button class="wizard-nav-btn primary" id="wizardFinish" style="display: none;">
                            Finish <span>✓</span>
                        </button>
                        <div class="wizard-divider"></div>
                        <button class="wizard-control-btn maximize-btn" id="wizardMaximize"
                                title="Maximize" aria-label="Maximize wizard">
                            <span>⛶</span>
                        </button>
                        <button class="wizard-control-btn close-btn" id="wizardClose"
                                title="Close" aria-label="Close wizard">
                            <span>×</span>
                        </button>
                    </div>

                    <!-- Main Content Container (Row Layout) -->
                    <div class="wizard-content-container">
                        <!-- Creative Left Sidebar -->
                        <div class="wizard-sidebar-creative">
                            <!-- Header with gradient accent -->
                            <div class="wizard-sidebar-header-creative">
                                <div class="wizard-brand">
                                    <div class="wizard-logo">
                                        <span class="logo-icon">🔗</span>
                                        <div class="logo-pulse"></div>
                                    </div>
                                    <div class="wizard-title-creative">
                                        <h2>Integration</h2>
                                        <span class="subtitle">Interface Wizard</span>
                                    </div>
                                </div>
                            </div>

                            <!-- Creative Progress with Animated Path -->
                            <div class="wizard-progress-creative">
                                <div class="progress-path">
                                    <svg class="progress-svg" viewBox="0 0 300 400">
                                        <path class="progress-track" d="M50 50 Q250 50 250 150 Q250 250 50 250 Q50 350 250 350"
                                              fill="none" stroke="rgba(255,255,255,0.2)" stroke-width="3"/>
                                        <path class="progress-fill" d="M50 50 Q250 50 250 150 Q250 250 50 250 Q50 350 250 350"
                                              fill="none" stroke="#f8bbd9" stroke-width="3" stroke-dasharray="1000" stroke-dashoffset="1000"/>
                                    </svg>
                                </div>

                                <!-- Creative Steps -->
                                <div class="wizard-steps-creative" role="tablist" aria-label="Wizard steps">
                                    ${this.getCreativeStepsIndicator()}
                                </div>
                            </div>

                            <!-- Bottom accent -->
                            <div class="sidebar-accent">
                                <div class="accent-pattern"></div>
                            </div>
                        </div>

                        <!-- Main Content Area -->
                        <div class="wizard-main">
                            <div class="wizard-step-container" id="wizardStepContainer">
                                <!-- Dynamic step content -->
                            </div>
                        </div>
                    </div>

                    <!-- Footer -->
                    <div class="wizard-footer">
                        <div class="wizard-footer-content">
                            <div class="wizard-validation-summary" id="wizardValidation">
                                <!-- Validation messages -->
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        `;
    }

    /**
     * Generate step indicator
     */
    getStepsIndicator() {
        const steps = [
            { number: 1, title: 'Configuration', icon: '⚙️' },
            { number: 2, title: 'HL7 Parsing', icon: '🔍' },
            { number: 3, title: 'FHIR Transform', icon: '🔄' },
            { number: 4, title: 'Target Config', icon: '🎯' }
        ];

        return steps.map(step => `
            <div class="wizard-step ${step.number === 1 ? 'active' : ''}"
                 id="wizardStep${step.number}"
                 role="tab"
                 aria-selected="${step.number === 1}"
                 tabindex="${step.number === 1 ? '0' : '-1'}">
                <div class="wizard-step-indicator">
                    <span class="wizard-step-icon">${step.icon}</span>
                    <span class="wizard-step-number">${step.number}</span>
                </div>
                <div class="wizard-step-title">${step.title}</div>
            </div>
        `).join('');
    }

    /**
     * Generate creative steps indicator
     */
    getCreativeStepsIndicator() {
        const steps = [
            { number: 1, title: 'Configuration', icon: '⚙️', description: 'Interface settings' },
            { number: 2, title: 'Source Setup', icon: '🔍', description: 'Message analysis' },
            { number: 3, title: 'Processing', icon: '🔄', description: 'Transform & mapping' },
            { number: 4, title: 'Target Config', icon: '🎯', description: 'Endpoint setup' }
        ];

        return steps.map((step, index) => `
            <div class="wizard-step-creative ${step.number === 1 ? 'active' : ''}"
                 data-step="${step.number}" tabindex="-1"
                 role="tab" aria-selected="${step.number === 1}"
                 style="--step-delay: ${index * 150}ms">
                <div class="step-connector"></div>
                <div class="step-bubble">
                    <div class="step-icon">${step.icon}</div>
                    <div class="step-number">${step.number}</div>
                    <div class="step-glow"></div>
                </div>
                <div class="step-content">
                    <div class="step-title">${step.title}</div>
                    <div class="step-description">${step.description}</div>
                </div>
            </div>
        `).join('');
    }

    /**
     * Render specific step content
     */
    renderStep(stepNumber, data = {}) {
        console.log('🎨 renderStep called for step', stepNumber, 'with data:', data);

        if (this.isAnimating) {
            console.log('⏸️ Animation in progress, skipping render');
            return;
        }

        // Safety check: if data is empty object, try to get it from controller
        if (Object.keys(data).length === 0 && window.wizardController) {
            console.warn('⚠️ Empty data passed to renderStep, fetching from controller...');
            data = window.wizardController.getCurrentData();
            console.log('✅ Retrieved data from controller:', Object.keys(data));
        }

        const stepContainer = document.getElementById('wizardStepContainer');
        if (!stepContainer) {
            console.error('❌ wizardStepContainer not found!');
            return;
        }

        console.log('📝 Generating template for step', stepNumber);
        const stepContent = this.getStepTemplate(stepNumber, data);
        console.log('📄 Generated template length:', stepContent.length);

        // Smooth transition
        this.animateStepTransition(stepContainer, stepContent);
        this.updateStepIndicator(stepNumber);
        this.updateNavigation(stepNumber, data.validation);
        this.updateHelp(stepNumber);

        // Validate the step after rendering to update button states
        setTimeout(() => {
            console.log('⚡ Running post-render validation...');
            this.validateCurrentStep();
        }, 150);
    }

    /**
     * Animate step transition
     */
    async animateStepTransition(container, newContent) {
        console.log('🎬 Starting step transition animation');
        this.isAnimating = true;

        // Fade out current content
        container.style.opacity = '0';
        container.style.transform = 'translateX(-20px)';

        await this.sleep(this.animationDuration / 2);

        // Update content
        console.log('📝 Setting container innerHTML');
        container.innerHTML = newContent;

        // Fade in new content
        container.style.opacity = '1';
        container.style.transform = 'translateX(0)';

        await this.sleep(this.animationDuration / 2);
        this.isAnimating = false;

        console.log('✅ Step transition animation complete');

        // Setup step-specific event listeners
        // Find the step content element with data-step attribute
        const stepContent = container.querySelector('[data-step]');
        if (stepContent) {
            console.log('🎯 Found step content for event listeners:', {
                tagName: stepContent.tagName,
                dataStep: stepContent.getAttribute('data-step'),
                className: stepContent.className
            });
            this.setupStepEventListeners(stepContent);
        } else {
            console.warn('⚠️ No step content with data-step attribute found in container');
        }
    }

    /**
     * Get step template based on step number
     */
    getStepTemplate(stepNumber, data) {
        const cacheKey = `step${stepNumber}`;

        switch (stepNumber) {
            case 1:
                return this.getStep1Template(data);
            case 2:
                return this.getStep2Template(data); // HL7 Parsing (old step 4)
            case 3:
                return this.getStep4Template(data); // FHIR Transform
            case 4:
                return this.getStep3Template(data); // Target Config
            default:
                return '<div class="wizard-error">Invalid step</div>';
        }
    }

    /**
     * Step 1: Basic Information
     */
    getStep1Template(data) {
        return `
            <div class="wizard-step-content" data-step="1">
                <div class="wizard-step-header">
                    <h3>Interface Configuration</h3>
                    <p>Set up basic information and source configuration</p>
                </div>

                <div class="wizard-form">
                    <!-- Basic Information Section -->
                    <div class="config-section">
                        <h4 class="section-title">📝 Basic Information</h4>
                        <div class="form-row">
                            <div class="form-group">
                                <label for="interfaceName" class="form-label required">Interface Name</label>
                                <input type="text"
                                       id="interfaceName"
                                       class="form-control"
                                       placeholder="e.g., Main Hospital ADT Interface"
                                       value="${data.name || ''}"
                                       maxlength="100"
                                       required>
                                <div class="form-hint">Choose a descriptive name (3-100 characters)</div>
                            </div>
                            <div class="form-group">
                                <label for="interfaceDescription" class="form-label">Description</label>
                                <textarea id="interfaceDescription"
                                          class="form-control"
                                          rows="2"
                                          placeholder="Brief description (optional)"
                                          maxlength="500">${data.description || ''}</textarea>
                                <div class="form-hint">💡 Auto-generated if empty</div>
                            </div>
                        </div>
                    </div>

                    <!-- Source Configuration Section -->
                    <div class="config-section">
                        <h4 class="section-title">📥 Source Configuration</h4>
                        <div class="form-row">
                            <div class="form-group">
                                <label for="sourceType" class="form-label required">Source Format</label>
                                <select id="sourceType" class="form-control" required>
                                    <option value="hl7v2" ${data.sourceType === 'hl7v2' ? 'selected' : ''}>HL7 v2.x (Standard)</option>
                                    <option value="fhir" ${data.sourceType === 'fhir' ? 'selected' : ''}>FHIR</option>
                                    <option value="ccda" ${(data.sourceType === 'ccda' || data.sourceType === 'cda') ? 'selected' : ''}>CDA/CCD (C-CDA, C32)</option>
                                    <option value="file" ${data.sourceType === 'file' ? 'selected' : ''}>File-based</option>
                                </select>
                            </div>
                        </div>
                        <div id="wizardInboundConnectorContainer" class="wizard-embedded-connector"></div>
                    </div>

                    <!-- Transformation Flow Section -->
                    <div class="config-section">
                        <h4 class="section-title">🔄 Transformation Flow</h4>
                        <div class="form-group">
                            <label for="transformationFlow" class="form-label required">Select Processing Flow</label>
                            <select id="transformationFlow" class="form-control" required>
                                <option value="">Choose transformation flow...</option>
                                <optgroup label="🔀 Transformation Flows (Auto-processing)">
                                    <option value="hl7_to_fhir" ${data.transformationFlow === 'hl7_to_fhir' ? 'selected' : ''}>
                                        HL7 v2.x → FHIR R4 (Recommended)
                                    </option>
                                    <option value="hl7_to_fhir_r5" ${data.transformationFlow === 'hl7_to_fhir_r5' ? 'selected' : ''}>
                                        HL7 v2.x → FHIR R5 (Emerging Standard)
                                    </option>
                                    <option value="ccd_to_fhir" ${data.transformationFlow === 'ccd_to_fhir' ? 'selected' : ''}>
                                        CCD/C-CDA → FHIR R4 (Automatic Transformation)
                                    </option>
                                </optgroup>
                                <optgroup label="📦 Passthrough Flows (No transformation)">
                                    <option value="passthrough" ${data.transformationFlow === 'passthrough' ? 'selected' : ''}>
                                        Passthrough (Store only, no transformation)
                                    </option>
                                    <option value="fhir_receiver" ${data.transformationFlow === 'fhir_receiver' ? 'selected' : ''}>
                                        FHIR Receiver (Direct storage, user-driven)
                                    </option>
                                    <option value="file_processor" ${data.transformationFlow === 'file_processor' ? 'selected' : ''}>
                                        File Processor (Batch processing, user-driven)
                                    </option>
                                </optgroup>
                                <optgroup label="⚙️ Custom">
                                    <option value="others" ${data.transformationFlow === 'others' ? 'selected' : ''}>
                                        Other / Custom (Configure in Pipeline Builder)
                                    </option>
                                </optgroup>
                            </select>
                            <div class="form-hint">
                                ℹ️ Transformation flows automatically process messages. Passthrough flows store for manual processing.
                            </div>
                        </div>

                        <!-- Flow Description (Dynamic based on selection) -->
                        <div id="flowDescription" class="alert alert-info" style="display: none; margin-top: 12px; padding: 12px; background: #e3f2fd; border: 1px solid #90caf9; border-radius: 6px;">
                            <strong id="flowDescTitle"></strong>
                            <p id="flowDescText" style="margin: 8px 0 0 0; font-size: 13px;"></p>
                        </div>
                    </div>

                    <!-- Message Family Filter Section -->
                    <div class="config-section" id="wizardFamilyFilterSection" style="display:${(typeof getFlow !== 'undefined' ? getFlow(data.transformationFlow || 'hl7_to_fhir').showFamilyFilter : ['hl7_to_fhir','hl7_to_fhir_r5'].includes(data.transformationFlow || 'hl7_to_fhir')) ? 'block' : 'none'};">
                        <h4 class="section-title">🔀 Message Family Filter</h4>
                        <div style="background:linear-gradient(to right,#f0fdf4,#f0f9ff);border-left:3px solid #34d399;padding:14px;border-radius:6px;">
                            <p style="font-size:0.82rem;color:#6b7280;margin:0 0 10px;">
                                Restrict which HL7 message families this interface accepts. Leave unrestricted to accept all. Unmatched messages receive a NACK (AR).
                            </p>
                            <label style="display:flex;align-items:center;cursor:pointer;margin-bottom:12px;">
                                <input type="checkbox" id="wizardFamilyFilterEnabled"
                                       onchange="window._wizardToggleFamilyFilter(this.checked)"
                                       style="margin-right:8px;width:16px;height:16px;cursor:pointer;accent-color:#0369a1;"
                                       ${Array.isArray(data.acceptedMessageFamilies) && data.acceptedMessageFamilies.length > 0 ? 'checked' : ''}>
                                <span style="font-size:0.9rem;color:#1e3a8a;font-weight:500;">Restrict to specific message families</span>
                            </label>
                            <div id="wizardFamilyFilterPicker" style="display:${Array.isArray(data.acceptedMessageFamilies) && data.acceptedMessageFamilies.length > 0 ? 'block' : 'none'};">
                                <div style="font-size:0.79rem;color:#64748b;margin-bottom:10px;">Click to select. MFN events must be chosen individually.</div>
                                <div style="display:flex;flex-wrap:wrap;gap:6px;" id="wizardFamilyChips">
                                    ${['ADT','ORU','ORM','SIU','MDM','VXU','RDE','BAR','DFT'].map(f =>
                                        `<span class="family-chip${Array.isArray(data.acceptedMessageFamilies) && data.acceptedMessageFamilies.includes(f) ? ' selected' : ''}"
                                               data-family="${f}" onclick="window._wizardToggleFamilyChip('${f}')"
                                               title="${{ADT:'Admit/Discharge/Transfer',ORU:'Observation Results',ORM:'Order Entry',SIU:'Scheduling',MDM:'Medical Document',VXU:'Vaccination',RDE:'Pharmacy',BAR:'Billing',DFT:'Financial'}[f]}">${f}</span>`
                                    ).join('')}
                                    <span style="width:100%;font-size:0.75rem;color:#94a3b8;padding-top:4px;font-weight:500;">MFN — select individually:</span>
                                    ${['MFN^M02','MFN^M04','MFN^M05','MFN^M12','MFN^M13'].map(f => {
                                        const label = {MFN_M02:'Staff',MFN_M04:'Charge',MFN_M05:'Location',MFN_M12:'Observation',MFN_M13:'Generic'}[f.replace('^','_')];
                                        return `<span class="family-chip${Array.isArray(data.acceptedMessageFamilies) && data.acceptedMessageFamilies.includes(f) ? ' selected' : ''}"
                                                      data-family="${f}" onclick="window._wizardToggleFamilyChip('${f}')">${f} ${label}</span>`;
                                    }).join('')}
                                </div>
                            </div>
                            <input type="hidden" id="wizardAcceptedMessageFamilies"
                                   value="${Array.isArray(data.acceptedMessageFamilies) && data.acceptedMessageFamilies.length > 0 ? JSON.stringify(data.acceptedMessageFamilies).replace(/"/g,'&quot;') : ''}">
                        </div>
                    </div>

                    <!-- Deployment Settings Section -->
                    <div class="config-section">
                        <h4 class="section-title">🚀 Deployment Settings</h4>
                        <div class="form-group">
                            <label for="deploymentMode" class="form-label">Startup Mode</label>
                            <select id="deploymentMode" class="form-control">
                                <option value="manual" ${(data.deploymentMode || data.deployment_mode || 'manual') === 'manual' ? 'selected' : ''}>Manual - Start manually from UI</option>
                                <option value="auto" ${(data.deploymentMode || data.deployment_mode) === 'auto' ? 'selected' : ''}>Auto - Start automatically on service startup</option>
                                <option value="delayed" ${(data.deploymentMode || data.deployment_mode) === 'delayed' ? 'selected' : ''}>Delayed - Auto-start with delay</option>
                            </select>
                            <div class="form-hint">
                                💡 Manual: You control when interface starts<br>
                                ⚡ Auto: Interface starts automatically when service boots up<br>
                                ⏱️ Delayed: Auto-start after a delay (useful for dependencies)
                            </div>
                        </div>

                        <div id="delaySettingsPanel" style="display: ${(data.deploymentMode || data.deployment_mode) === 'delayed' ? 'block' : 'none'}; margin-top: 12px;">
                            <div class="form-group">
                                <label for="deploymentDelay" class="form-label">Delay (seconds)</label>
                                <input type="number"
                                       id="deploymentDelay"
                                       class="form-control"
                                       min="0"
                                       max="300"
                                       value="${data.deploymentDelay || data.deployment_delay_seconds || 0}"
                                       placeholder="0">
                                <div class="form-hint">Delay in seconds before auto-starting (0-300 seconds)</div>
                            </div>
                        </div>

                        <div class="form-group" style="margin-top: 12px;">
                            <label class="form-label" style="display: flex; align-items: center; cursor: pointer;">
                                <input type="checkbox"
                                       id="autoStart"
                                       style="margin-right: 8px; width: 18px; height: 18px; cursor: pointer;"
                                       ${data.autoStart || data.auto_start ? 'checked' : ''}>
                                <span>Auto-start interface after creation (start immediately after wizard completes)</span>
                            </label>
                            <div class="form-hint">
                                ✅ Recommended for development/testing<br>
                                ⚠️ Disable for production (verify configuration first)
                            </div>
                        </div>
                    </div>

                    <!-- Logging & Troubleshooting Section -->
                    <div class="config-section" style="margin-top: 1.5rem;">
                        <h4 class="section-title">📋 Logging & Troubleshooting</h4>

                        <!-- Debug Logging Toggle -->
                        <div class="form-group">
                            <div style="background: linear-gradient(to right, #fef2f2, #fff7ed); border-left: 3px solid #f59e0b; padding: 14px; border-radius: 6px;">
                                <label class="form-label" style="display: flex; align-items: center; cursor: pointer; margin: 0;">
                                    <input type="checkbox"
                                           id="debugLogging"
                                           style="margin-right: 10px; width: 20px; height: 20px; cursor: pointer; accent-color: #f59e0b;"
                                           ${data.debugLogging || data.debug_logging ? 'checked' : ''}>
                                    <div style="flex: 1;">
                                        <span style="font-weight: 600; color: #1e3a8a; font-size: 0.95rem;">Enable Debug Logging</span>
                                        <div style="font-size: 0.85rem; color: #6b7280; margin-top: 4px; line-height: 1.4;">
                                            📝 Captures detailed logs for all message processing operations
                                        </div>
                                        <div style="font-size: 0.85rem; color: #dc2626; margin-top: 4px; font-weight: 500;">
                                            ⚠️ Warning: Increases storage usage
                                        </div>
                                    </div>
                                </label>
                            </div>
                        </div>

                        <!-- Log Retention Period -->
                        <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 16px; margin-top: 16px;">
                            <div class="form-group" style="margin: 0;">
                                <label for="logRetentionDays" class="form-label" style="font-weight: 600; color: #1e3a8a; margin-bottom: 8px;">
                                    🗑️ Log Retention Period
                                </label>
                                <div style="display: flex; align-items: center; gap: 8px;">
                                    <input type="number"
                                           id="logRetentionDays"
                                           class="form-control"
                                           min="1"
                                           max="365"
                                           value="${data.logRetentionDays || data.log_retention_days || 30}"
                                           placeholder="30"
                                           style="max-width: 100px; text-align: center; font-weight: 600;">
                                    <span style="color: #6b7280; font-size: 0.9rem;">days</span>
                                </div>
                                <div class="form-hint" style="margin-top: 6px;">
                                    Debug/info logs auto-deleted after this period
                                </div>
                            </div>

                            <!-- Retain Errors Toggle -->
                            <div class="form-group" style="margin: 0;">
                                <label class="form-label" style="font-weight: 600; color: #1e3a8a; margin-bottom: 8px;">
                                    ♾️ Error Log Retention
                                </label>
                                <div style="background: #f0f9ff; border: 1px solid #bfdbfe; padding: 10px 12px; border-radius: 6px;">
                                    <label style="display: flex; align-items: center; cursor: pointer; margin: 0;">
                                        <input type="checkbox"
                                               id="retainErrorLogs"
                                               style="margin-right: 8px; width: 18px; height: 18px; cursor: pointer; accent-color: #0369a1;"
                                               ${data.retainErrorLogs !== false && data.retain_error_logs_forever !== false ? 'checked' : ''}>
                                        <span style="font-size: 0.9rem; color: #1e3a8a; font-weight: 500;">Keep errors forever</span>
                                    </label>
                                </div>
                                <div class="form-hint" style="margin-top: 6px;">
                                    ✅ Recommended for compliance
                                </div>
                            </div>
                        </div>

                        <!-- Info Panel -->
                        <div style="background: #f9fafb; border: 1px solid #e5e7eb; padding: 12px; border-radius: 6px; margin-top: 16px;">
                            <div style="font-size: 0.875rem; color: #374151; line-height: 1.6;">
                                <strong style="color: #1e3a8a;">Retention Policy Summary:</strong><br>
                                • <strong>Debug/Info logs:</strong> Deleted after <span id="retentionSummary" style="color: #0369a1; font-weight: 600;">${data.logRetentionDays || data.log_retention_days || 30} days</span><br>
                                • <strong>Error/Warning logs:</strong> <span id="errorRetentionSummary" style="color: #059669; font-weight: 600;">${data.retainErrorLogs !== false && data.retain_error_logs_forever !== false ? 'Kept forever' : 'Same as debug logs'}</span><br>
                                • <strong>Audit logs:</strong> <span style="color: #059669; font-weight: 600;">Always retained (HIPAA compliance)</span>
                            </div>
                        </div>
                    </div>

                </div>
            </div>
        `;
    }

    /**
     * Step 2: HL7 Parsing & Sample
     */
    getStep2Template(data) {
        return `
            <div class="wizard-step-content" data-step="2">
                <div class="wizard-step-header">
                    <h3>HL7 Message Parsing & Sample <span style="font-size:14px;font-weight:400;color:#6b7280;vertical-align:middle;">(Optional)</span></h3>
                    <p>Provide a sample HL7 message for a more accurate field mapping preview, or skip to use an auto-loaded default</p>
                </div>

                <!-- Optional step notice -->
                <div style="background: linear-gradient(to right, #fffbeb, #fef9c3); border: 1px solid #fde68a; border-radius: 8px; padding: 14px 16px; margin-bottom: 16px;">
                    <div style="display:flex;align-items:flex-start;justify-content:space-between;gap:16px;">
                        <div>
                            <div style="display:flex;align-items:center;gap:8px;margin-bottom:6px;">
                                <span style="font-size:16px;">💡</span>
                                <strong style="font-size:13px;color:#92400e;">This step is optional — you can skip and proceed with auto-generated mappings</strong>
                            </div>
                            <ul style="margin:0;padding-left:20px;font-size:12px;color:#78350f;line-height:1.8;">
                                <li><strong>Skip:</strong> Built-in sample messages are used to preview FHIR mappings. Fast and works for most standard interfaces.</li>
                                <li><strong>Use your own sample:</strong> Paste a real HL7 message from your system to see mappings tailored to your exact segment structure, custom Z-segments, and field values.</li>
                                <li><strong>HL7 version matters:</strong> If your system sends v2.3 or v2.4 messages (vs the default v2.5 samples), upload one to get accurate version-specific mappings.</li>
                            </ul>
                        </div>
                        <button type="button"
                                onclick="window._skipWizardHL7Step && window._skipWizardHL7Step()"
                                style="flex-shrink:0;padding:7px 18px; border: 2px solid #f59e0b; border-radius:6px; background:white; color:#b45309; font-size:13px; font-weight:600; cursor:pointer; white-space:nowrap;">
                            Skip →
                        </button>
                    </div>
                </div>

                <div class="wizard-form">
                    <!-- HL7 Input Options -->
                    <div class="hl7-input-section">
                        <div class="input-options">
                            <div class="option-card">
                                <h4>📝 Paste HL7 Message</h4>
                                <p>Paste your own HL7 message for parsing</p>
                                <div class="form-group">
                                    <label for="hl7Message" class="form-label">HL7 Message Content</label>
                                    <textarea id="hl7Message" class="form-control hl7-textarea"
                                              rows="8" placeholder="MSH|^~\\&|HIS|RIH|EKG|EKG~ECG~EKG2|199904140038||ADT^A01|12345|P|2.5&#10;PID|1||123456||DOE^JOHN^||19800101|M|||123 MAIN ST^^ANYTOWN^NY^12345|||||S||123456789|123456789&#10;PV1|1|I|2000^2012^01||||004777^ATTEND^AARON^A|||SUR|||||||004777^ATTEND^AARON^A|INP|INS|||19800101140038|||||||||||||||||||">${data.hl7Message || ''}</textarea>
                                </div>
                                <div class="parse-options-row" style="display:flex;align-items:center;gap:16px;margin-bottom:10px;">
                                    <button type="button" class="btn btn-primary" id="parseHL7Btn">
                                        <span class="btn-icon">🔍</span>
                                        Parse HL7 Message
                                    </button>
                                    <label style="display:flex;align-items:center;gap:6px;font-size:0.85em;color:#555;cursor:pointer;" title="When enabled, escape sequences like \\F\\ \\S\\ \\T\\ \\R\\ \\E\\ are converted to their literal characters during parsing.">
                                        <input type="checkbox" id="decodeEscapes" ${data.decodeEscapes ? 'checked' : ''} style="cursor:pointer;">
                                        Decode escape sequences
                                        <span style="color:#888;font-size:0.9em;">(\\F\\→|, \\S\\→^, \\T\\→&amp;, \\R\\→~, \\E\\→\\)</span>
                                    </label>
                                </div>
                            </div>

                            <div class="option-divider">
                                <span>OR</span>
                            </div>

                            <div class="option-card">
                                <h4>🚀 Use Sample HL7</h4>
                                <p>Select from pre-built sample messages</p>
                                <div class="sample-options">
                                    <button type="button" class="btn btn-outline-primary sample-btn" data-message-type="ADT^A01">
                                        ADT^A01 - Patient Admission
                                    </button>
                                    <button type="button" class="btn btn-outline-primary sample-btn" data-message-type="ORU^R01">
                                        ORU^R01 - Lab Results
                                    </button>
                                    <button type="button" class="btn btn-outline-primary sample-btn" data-message-type="ADT^A03">
                                        ADT^A03 - Patient Discharge
                                    </button>
                                </div>
                            </div>
                        </div>
                    </div>

                    <!-- Parsing Results -->
                    <div class="parsing-results" id="parsingResults" style="display: ${data.parsedHL7Data ? 'block' : 'none'}">
                        ${data.parsedHL7Data ? this.getTransformationResultsTemplate(data) : ''}
                    </div>
                </div>
            </div>
        `;
    }

    /**
     * Get source configuration panel based on connectivity type
     * REFACTORED: Now delegates to shared InterfaceConfigComponents
     */
    getSourceConfigPanel(connectivity, config = {}, sourceType = '') {
        // Delegate to shared component library
        return InterfaceConfigComponents.getSourceConfigPanel(
            connectivity,
            sourceType,
            config,
            { idPrefix: '' }  // Wizard uses no prefix for backward compatibility
        );
    }

    /**
     * Get FHIR Receiver specific configuration panel
     * REFACTORED: Delegates to shared InterfaceConfigComponents
     */
    getFhirReceiverConfig(config = {}) {
        return InterfaceConfigComponents.getFhirReceiverConfig(config, '');
    }

    /**
     * LEGACY METHOD - Kept for backward compatibility but now delegates to shared component
     */
    _legacyGetFhirReceiverConfig(config = {}) {
        return `
            <div class="config-group fhir-receiver-config">
                <h4>🏥 FHIR Receiver Configuration</h4>

                <!-- Base URL Path -->
                <div class="form-group">
                    <label for="fhirBasePath" class="form-label required">Base URL Path</label>
                    <input type="text" id="fhirBasePath" class="form-control"
                           value="${config.basePath || '/fhir'}"
                           placeholder="/fhir">
                    <div class="form-hint">💡 Example endpoint: <code>/fhir/receiver/{interfaceId}</code> or <code>/fhir/{resourceType}</code></div>
                </div>

                <!-- FHIR Version -->
                <div class="form-group">
                    <label for="fhirVersion" class="form-label required">FHIR Version</label>
                    <select id="fhirVersion" class="form-control">
                        <option value="R4" ${!config.fhirVersion || config.fhirVersion === 'R4' ? 'selected' : ''}>FHIR R4 (Recommended)</option>
                        <option value="R5" ${config.fhirVersion === 'R5' ? 'selected' : ''}>FHIR R5 (Emerging Standard)</option>
                    </select>
                    <div class="form-hint">Select the FHIR version your destination system expects</div>
                </div>

                <!-- Supported Operations -->
                <div class="form-group">
                    <label class="form-label">
                        Supported REST Operations
                        <a href="#" class="help-link" onclick="event.preventDefault(); AppDialogs.alert('Choose which FHIR REST API operations to enable:<br><br>• <b>CREATE (POST)</b> — Most common, create new resources<br>• <b>READ (GET)</b> — Retrieve resources by ID<br>• <b>UPDATE (PUT)</b> — Replace entire resource<br>• <b>PATCH</b> — Partial updates<br>• <b>DELETE</b> — Remove resources (use with caution)<br>• <b>SEARCH</b> — Query with parameters<br>• <b>BATCH/TRANSACTION</b> — Multiple operations in one request<br><br><i>Recommendation: Start with CREATE only, add others as needed.</i>', { title: 'FHIR Operations Help', type: 'info' });" title="Click for help">ⓘ</a>
                    </label>
                    <div class="checkbox-group">
                        <label class="checkbox-label">
                            <input type="checkbox" id="opCreate" ${!config.operations || config.operations.includes('CREATE') ? 'checked' : ''}>
                            <span>CREATE (POST)</span>
                        </label>
                        <label class="checkbox-label">
                            <input type="checkbox" id="opRead" ${config.operations?.includes('READ') ? 'checked' : ''}>
                            <span>READ (GET)</span>
                        </label>
                        <label class="checkbox-label">
                            <input type="checkbox" id="opUpdate" ${config.operations?.includes('UPDATE') ? 'checked' : ''}>
                            <span>UPDATE (PUT)</span>
                        </label>
                        <label class="checkbox-label">
                            <input type="checkbox" id="opPatch" ${config.operations?.includes('PATCH') ? 'checked' : ''}>
                            <span>PATCH</span>
                        </label>
                        <label class="checkbox-label">
                            <input type="checkbox" id="opDelete" ${config.operations?.includes('DELETE') ? 'checked' : ''}>
                            <span>DELETE</span>
                        </label>
                        <label class="checkbox-label">
                            <input type="checkbox" id="opSearch" ${config.operations?.includes('SEARCH') ? 'checked' : ''}>
                            <span>SEARCH</span>
                        </label>
                        <label class="checkbox-label">
                            <input type="checkbox" id="opBatch" ${config.operations?.includes('BATCH') ? 'checked' : ''}>
                            <span>BATCH/TRANSACTION</span>
                        </label>
                    </div>
                </div>

                <!-- Resource Filtering -->
                <div class="form-group">
                    <label class="checkbox-label">
                        <input type="checkbox" id="enableResourceFilter" ${config.enableResourceFilter ? 'checked' : ''}>
                        <span>Restrict Accepted Resource Types</span>
                    </label>
                    <div id="resourceFilterPanel" style="display: ${config.enableResourceFilter ? 'block' : 'none'}; margin-top: 12px;">
                        <div class="checkbox-group" style="max-height: 200px; overflow-y: auto; border: 1px solid #ddd; padding: 12px; border-radius: 4px;">
                            ${this.getFhirResourceCheckboxes(config.acceptedResources || [])}
                        </div>
                        <label class="checkbox-label" style="margin-top: 8px;">
                            <input type="checkbox" id="rejectUnknownResources" ${config.rejectUnknownResources ? 'checked' : ''}>
                            <span>Reject resources not in the list above</span>
                        </label>
                    </div>
                </div>

                <!-- Validation Settings -->
                <div class="form-group">
                    <label class="form-label">Validation Settings</label>
                    <label class="checkbox-label">
                        <input type="checkbox" id="validateStructure" ${config.validateStructure !== false ? 'checked' : ''}>
                        <span>Validate Resource Structure (required fields, data types)</span>
                    </label>
                    <label class="checkbox-label">
                        <input type="checkbox" id="validateProfiles" ${config.validateProfiles ? 'checked' : ''}>
                        <span>Validate Against Profiles (US Core, IPS, etc.)</span>
                    </label>
                    <label class="checkbox-label">
                        <input type="checkbox" id="validateTerminology" ${config.validateTerminology ? 'checked' : ''}>
                        <span>Validate Terminology (CodeSystems, ValueSets)</span>
                    </label>
                    <label class="checkbox-label">
                        <input type="checkbox" id="rejectInvalid" ${config.rejectInvalid ? 'checked' : ''}>
                        <span>Reject Invalid Resources (vs Accept with Warnings)</span>
                    </label>
                </div>

                <!-- Rate Limiting -->
                <div class="form-group">
                    <label class="checkbox-label">
                        <input type="checkbox" id="enableRateLimit" ${config.enableRateLimit ? 'checked' : ''}>
                        <span>Enable Rate Limiting</span>
                    </label>
                    <div id="rateLimitPanel" style="display: ${config.enableRateLimit ? 'block' : 'none'}; margin-top: 12px;">
                        <div class="form-row">
                            <div class="form-group">
                                <label for="rateLimit" class="form-label">Requests per Minute</label>
                                <input type="number" id="rateLimit" class="form-control"
                                       value="${config.rateLimit || 60}"
                                       min="1" max="10000">
                            </div>
                            <div class="form-group">
                                <label for="burstLimit" class="form-label">Burst Allowance</label>
                                <input type="number" id="burstLimit" class="form-control"
                                       value="${config.burstLimit || 10}"
                                       min="1" max="1000">
                            </div>
                        </div>
                    </div>
                </div>

                <!-- Content Format -->
                <div class="form-group">
                    <label class="form-label">Accepted Content Types</label>
                    <label class="checkbox-label">
                        <input type="checkbox" id="acceptFhirJson" ${config.acceptFhirJson !== false ? 'checked' : ''}>
                        <span>application/fhir+json (FHIR Standard)</span>
                    </label>
                    <label class="checkbox-label">
                        <input type="checkbox" id="acceptJson" ${config.acceptJson !== false ? 'checked' : ''}>
                        <span>application/json (Fallback)</span>
                    </label>
                    <label class="checkbox-label">
                        <input type="checkbox" id="acceptFhirXml" ${config.acceptFhirXml ? 'checked' : ''}>
                        <span>application/fhir+xml (XML Support)</span>
                    </label>
                </div>

                <!-- Post-Reception Actions (Integration Engine Features) -->
                <div class="form-group" style="border-top: 2px solid #e0e0e0; padding-top: 20px; margin-top: 20px;">
                    <h4 style="margin-bottom: 16px;">🔄 Post-Reception Actions</h4>
                    <div class="form-hint" style="margin-bottom: 12px;">Configure what happens after receiving FHIR resources</div>

                    <label class="checkbox-label">
                        <input type="checkbox" id="actionStoreOnly" ${!config.postActions || config.postActions.includes('store') ? 'checked' : ''}>
                        <span>Store in Database (Always enabled)</span>
                    </label>

                    <label class="checkbox-label">
                        <input type="checkbox" id="actionTransform" ${config.postActions?.includes('transform') ? 'checked' : ''}>
                        <span>Apply Transformation Pipeline (custom mappings, enrichment)</span>
                    </label>

                    <label class="checkbox-label">
                        <input type="checkbox" id="actionForward" ${config.postActions?.includes('forward') ? 'checked' : ''}>
                        <span>Forward to Destination (route to another FHIR server)</span>
                    </label>

                    <label class="checkbox-label">
                        <input type="checkbox" id="actionWorkflow" ${config.postActions?.includes('workflow') ? 'checked' : ''}>
                        <span>Trigger Workflow (conditional routing, notifications)</span>
                    </label>

                    <label class="checkbox-label">
                        <input type="checkbox" id="actionAudit" ${config.postActions?.includes('audit') ? 'checked' : ''}>
                        <span>Generate FHIR AuditEvent (compliance tracking)</span>
                    </label>

                    <!-- Forwarding Configuration (shown when forward is checked) -->
                    <div id="forwardingConfig" style="display: ${config.postActions?.includes('forward') ? 'block' : 'none'}; margin-top: 12px; padding: 12px; background: #f5f5f5; border-radius: 4px;">
                        <div class="form-group">
                            <label for="forwardDestination" class="form-label">Forward Destination URL</label>
                            <input type="url" id="forwardDestination" class="form-control"
                                   value="${config.forwardDestination || ''}"
                                   placeholder="https://fhir-server.example.com/fhir">
                        </div>
                        <label class="checkbox-label">
                            <input type="checkbox" id="forwardAsync" ${config.forwardAsync !== false ? 'checked' : ''}>
                            <span>Async Forwarding (don't wait for response)</span>
                        </label>
                        <label class="checkbox-label">
                            <input type="checkbox" id="forwardOnlyValid" ${config.forwardOnlyValid ? 'checked' : ''}>
                            <span>Forward Only Valid Resources</span>
                        </label>
                    </div>
                </div>

                <!-- HTTP Authentication (Universal for all HTTP connections) -->
                ${this.getHttpAuthConfig(config)}

            </div>
        `;
    }

    /**
     * Get FHIR resource type checkboxes
     * REFACTORED: Delegates to shared InterfaceConfigComponents
     */
    getFhirResourceCheckboxes(selectedResources = []) {
        return InterfaceConfigComponents.getFhirResourceCheckboxes(selectedResources, '');
    }

    /**
     * Get HTTP Authentication Configuration Panel
     * REFACTORED: Delegates to shared InterfaceConfigComponents
     */
    getHttpAuthConfig(config = {}) {
        return InterfaceConfigComponents.getHttpAuthConfig(config, '');
    }

    /**
     * Get HTTP Authentication Details Panel
     * REFACTORED: Delegates to shared InterfaceConfigComponents
     */
    getHttpAuthDetailsPanel(authType, config = {}) {
        return InterfaceConfigComponents.getHttpAuthDetailsPanel(authType, config, '');
    }

    /**
     * Step 3: Target Configuration
     */
    getStep3Template(data) {
        // Check if this is a sink-only flow
        const isSinkOnly = this.isSinkOnlyFlow(data.transformationFlow);

        // Auto-set defaults for sink-only flows
        if (isSinkOnly) {
            data.targetConnectivity = 'sink';
            data.targetType = data.targetType || 'fhir';
        }

        return `
            <div class="wizard-step-content" data-step="4">
                <div class="wizard-step-header">
                    <h3>Target Configuration</h3>
                    <p>${isSinkOnly ? 'Messages will be stored in database only (no external routing)' : 'Configure where FHIR resources will be sent'}</p>
                </div>

                <div class="wizard-form">
                    ${isSinkOnly ? `
                        <!-- Sink-Only Flow: Show informational message only -->
                        <div class="alert alert-info" style="background: linear-gradient(135deg, #e3f2fd 0%, #ffffff 100%); padding: 24px; border: 2px solid #90caf9; border-radius: 12px; margin-bottom: 20px;">
                            <div style="display: flex; align-items: start; gap: 16px;">
                                <div style="font-size: 48px; line-height: 1;">📦</div>
                                <div style="flex: 1;">
                                    <h4 style="margin: 0 0 12px 0; color: #1976d2; font-size: 18px; font-weight: 600;">Sink Mode - Storage Only</h4>
                                    <p style="margin: 0 0 16px 0; color: #424242; font-size: 14px; line-height: 1.6;">
                                        Messages are received and stored in your database (<strong>PostgreSQL</strong>) but <strong>NOT routed</strong> to any external system.
                                    </p>
                                    <div style="background: white; padding: 16px; border-radius: 8px; border-left: 4px solid #4caf50;">
                                        <strong style="color: #2e7d32; display: block; margin-bottom: 8px;">✅ Perfect for:</strong>
                                        <ul style="margin: 0; padding-left: 20px; color: #424242; font-size: 13px; line-height: 1.8;">
                                            <li>FHIR receiving and storage</li>
                                            <li>Archival and audit purposes</li>
                                            <li>Manual processing workflows</li>
                                            <li>Development and testing</li>
                                        </ul>
                                    </div>
                                </div>
                            </div>
                        </div>

                        <!-- Hidden inputs to set connectivity as sink -->
                        <input type="hidden" id="targetConnectivity" value="sink">
                        <input type="hidden" id="targetType" value="${data.targetType || 'fhir'}">
                    ` : `
                        <!-- Outbound connector — same ConnectorConfigBuilder used by pipeline builder -->
                        <div id="wizardOutboundConnectorContainer" class="wizard-embedded-connector"></div>
                    `}
                </div>
            </div>
        `;
    }

    /**
     * Check if this is a sink-only flow (no external routing).
     * Delegates to FlowRegistry when available.
     */
    isSinkOnlyFlow(transformationFlow) {
        if (typeof getFlow !== 'undefined') {
            return getFlow(transformationFlow).isSinkOnly;
        }
        // Fallback (FlowRegistry not yet loaded)
        return ['passthrough', 'fhir_receiver', 'file_processor', 'others'].includes(transformationFlow);
    }

    /**
     * Check if delivery mode options should be shown based on selected transformation flow.
     * Delegates to FlowRegistry when available.
     */
    shouldShowDeliveryMode(transformationFlow) {
        if (typeof getFlow !== 'undefined') {
            return getFlow(transformationFlow).hasDelivery;
        }
        // Fallback
        return ['hl7_to_fhir', 'hl7_to_fhir_r5', 'ccd_to_fhir'].includes(transformationFlow);
    }

    /**
     * Get target configuration panel
     */
    getTargetConfigPanel(connectivity, config = {}) {
        // Delegate to shared components for all target connectivity types
        const targetType = document.querySelector('#targetType')?.value || 'fhir';
        return InterfaceConfigComponents.getTargetConfigPanel(connectivity, targetType, config, { idPrefix: '' });

        /* OLD IMPLEMENTATION - Replaced with shared components
        switch (connectivity) {
            case 'http':
                // Get transformation flow from dropdown (if available) or wizard data
                const transformationFlowDropdown = document.querySelector('#transformationFlow');
                const wizardData = this.controller?.model?.data || {};
                const transformationFlow = transformationFlowDropdown?.value || wizardData.transformationFlow || config.transformationFlow;
                const showDeliveryMode = this.shouldShowDeliveryMode(transformationFlow);

                console.log('🔍 Target config panel - DEBUG:', {
                    dropdownValue: transformationFlowDropdown?.value,
                    wizardDataFlow: wizardData.transformationFlow,
                    configFlow: config.transformationFlow,
                    finalFlow: transformationFlow,
                    showDeliveryMode: showDeliveryMode
                });

                return `
                    <div class="config-group">
                        <h4>FHIR Server Configuration</h4>
                        <div class="form-group">
                            <label for="targetEndpoint" class="form-label required">FHIR Base URL</label>
                            <input type="url" id="targetEndpoint" class="form-control"
                                   value="${config.endpoint || 'http://localhost:8080/fhir'}"
                                   placeholder="http://localhost:8080/fhir">
                            <div class="form-hint">💡 OOB: Local HAPI FHIR server</div>
                        </div>
                        <div class="form-row">
                            <div class="form-group">
                                <label for="targetVersion" class="form-label">FHIR Version</label>
                                <select id="targetVersion" class="form-control">
                                    <option value="R4" ${!config.version || config.version === 'R4' ? 'selected' : ''}>R4 (Recommended)</option>
                                    <option value="R5" ${config.version === 'R5' ? 'selected' : ''}>R5 (Emerging Standard)</option>
                                </select>
                            </div>
                            <div class="form-group">
                                <label for="targetFormat" class="form-label">Format</label>
                                <select id="targetFormat" class="form-control">
                                    <option value="json" ${config.format === 'json' ? 'selected' : ''}>JSON (Recommended)</option>
                                    <option value="xml" ${config.format === 'xml' ? 'selected' : ''}>XML</option>
                                </select>
                            </div>
                        </div>

                        <!-- Delivery Mode (Only for transformation flows) -->
                        ${showDeliveryMode ? `
                        <div class="form-group" id="deliveryModeGroup">
                            <label class="form-label">
                                Delivery Mode
                                <span class="badge badge-info" style="font-size: 10px; margin-left: 8px;">Transformation Flow</span>
                            </label>
                            <div class="form-row" style="gap: 10px;">
                                <div class="form-check" style="flex: 1;">
                                    <input type="radio" id="deliveryModeBundle" name="deliveryMode" value="bundle"
                                           class="form-check-input" ${!config.deliveryMode || config.deliveryMode === 'bundle' ? 'checked' : ''}>
                                    <label for="deliveryModeBundle" class="form-check-label">
                                        <strong>Bundle</strong>
                                        <div class="form-hint">Single transaction (Recommended)</div>
                                    </label>
                                </div>
                                <div class="form-check" style="flex: 1;">
                                    <input type="radio" id="deliveryModeIndividual" name="deliveryMode" value="individual"
                                           class="form-check-input" ${config.deliveryMode === 'individual' ? 'checked' : ''}>
                                    <label for="deliveryModeIndividual" class="form-check-label">
                                        <strong>Individual</strong>
                                        <div class="form-hint">Separate API calls per resource</div>
                                    </label>
                                </div>
                            </div>
                        </div>

                        <!-- Resource Selection (Only visible when Individual mode selected) -->
                        <div id="individualResourceSelection" class="form-group" style="display: ${config.deliveryMode === 'individual' ? 'block' : 'none'};">
                            <label class="form-label">Select Resources to Send</label>
                            <div class="form-hint" style="margin-bottom: 10px;">Choose which FHIR resources to send individually. Unselected resources will be skipped.</div>
                            <div class="resource-checkboxes" style="display: grid; grid-template-columns: repeat(3, 1fr); gap: 8px;">
                                <label class="form-check-label" style="display: flex; align-items: center; gap: 6px;">
                                    <input type="checkbox" name="individualResources" value="Patient" ${config.individualResources?.includes('Patient') !== false ? 'checked' : ''}>
                                    Patient
                                </label>
                                <label class="form-check-label" style="display: flex; align-items: center; gap: 6px;">
                                    <input type="checkbox" name="individualResources" value="Encounter" ${config.individualResources?.includes('Encounter') !== false ? 'checked' : ''}>
                                    Encounter
                                </label>
                                <label class="form-check-label" style="display: flex; align-items: center; gap: 6px;">
                                    <input type="checkbox" name="individualResources" value="Observation" ${config.individualResources?.includes('Observation') !== false ? 'checked' : ''}>
                                    Observation
                                </label>
                                <label class="form-check-label" style="display: flex; align-items: center; gap: 6px;">
                                    <input type="checkbox" name="individualResources" value="Condition" ${config.individualResources?.includes('Condition') !== false ? 'checked' : ''}>
                                    Condition
                                </label>
                                <label class="form-check-label" style="display: flex; align-items: center; gap: 6px;">
                                    <input type="checkbox" name="individualResources" value="Procedure" ${config.individualResources?.includes('Procedure') !== false ? 'checked' : ''}>
                                    Procedure
                                </label>
                                <label class="form-check-label" style="display: flex; align-items: center; gap: 6px;">
                                    <input type="checkbox" name="individualResources" value="MedicationRequest" ${config.individualResources?.includes('MedicationRequest') !== false ? 'checked' : ''}>
                                    MedicationRequest
                                </label>
                                <label class="form-check-label" style="display: flex; align-items: center; gap: 6px;">
                                    <input type="checkbox" name="individualResources" value="AllergyIntolerance" ${config.individualResources?.includes('AllergyIntolerance') !== false ? 'checked' : ''}>
                                    AllergyIntolerance
                                </label>
                                <label class="form-check-label" style="display: flex; align-items: center; gap: 6px;">
                                    <input type="checkbox" name="individualResources" value="DiagnosticReport" ${config.individualResources?.includes('DiagnosticReport') !== false ? 'checked' : ''}>
                                    DiagnosticReport
                                </label>
                                <label class="form-check-label" style="display: flex; align-items: center; gap: 6px;">
                                    <input type="checkbox" name="individualResources" value="MessageHeader" ${config.individualResources?.includes('MessageHeader') !== false ? 'checked' : ''}>
                                    MessageHeader
                                </label>
                            </div>
                            <div class="form-group" style="margin-top: 12px;">
                                <label class="form-check-label" style="display: flex; align-items: center; gap: 6px;">
                                    <input type="checkbox" id="stopOnIndividualError" ${config.stopOnIndividualError ? 'checked' : ''}>
                                    Stop processing if any resource fails
                                </label>
                            </div>
                        </div>

                        <!-- Default FHIR Operation -->
                        <div class="form-group">
                            <label for="defaultOperation" class="form-label">
                                Default FHIR Operation
                                <span class="badge badge-secondary" style="font-size: 10px; margin-left: 8px;">Applied to all resources</span>
                            </label>
                            <select id="defaultOperation" class="form-control">
                                <option value="POST" ${!config.defaultOperation || config.defaultOperation === 'POST' ? 'selected' : ''}>POST - Create new resource</option>
                                <option value="PUT" ${config.defaultOperation === 'PUT' ? 'selected' : ''}>PUT - Update existing resource</option>
                                <option value="PATCH" ${config.defaultOperation === 'PATCH' ? 'selected' : ''}>PATCH - Partial update</option>
                            </select>
                            <div class="form-hint" style="margin-top: 6px;">
                                POST creates new resources. PUT/PATCH require resource IDs from HL7 message.
                                <br><strong>Advanced routing logic</strong> (conditional operations, multiple destinations) can be configured in the Transformation Pipeline later.
                            </div>
                        </div>
                        ` : '<!-- Delivery mode hidden: Direct receiver or non-transformation flow -->'}

                        <!-- Authentication -->
                        <div class="form-group">
                            <label class="form-label">Authentication</label>
                            <div class="form-check">
                                <input type="checkbox" id="targetAuthEnabled" class="form-check-input"
                                       ${config.authEnabled ? 'checked' : ''}>
                                <label for="targetAuthEnabled" class="form-check-label">Enable Authentication</label>
                            </div>
                        </div>

                        <div id="authConfig" class="auth-config" ${!config.authEnabled ? 'style="display: none;"' : ''}>
                            <div class="form-group">
                                <label for="authType" class="form-label">Authentication Type</label>
                                <select id="authType" class="form-control">
                                    <option value="none" ${config.authType === 'none' || !config.authType ? 'selected' : ''}>None</option>
                                    <option value="basic" ${config.authType === 'basic' ? 'selected' : ''}>Basic Authentication</option>
                                    <option value="bearer" ${config.authType === 'bearer' ? 'selected' : ''}>Bearer Token</option>
                                    <option value="oauth2" ${config.authType === 'oauth2' ? 'selected' : ''}>OAuth 2.0 (Coming Soon)</option>
                                </select>
                            </div>

                            <!-- Basic Auth Fields (shown when authType = basic) -->
                            <div id="basicAuthFields" style="display: ${config.authType === 'basic' ? 'block' : 'none'};">
                                <div class="form-row">
                                    <div class="form-group">
                                        <label for="authUsername" class="form-label">Username</label>
                                        <input type="text" id="authUsername" class="form-control"
                                               value="${config.authUsername || ''}"
                                               placeholder="Enter username">
                                    </div>
                                    <div class="form-group">
                                        <label for="authPassword" class="form-label">Password</label>
                                        <input type="password" id="authPassword" class="form-control"
                                               value="${config.authPassword || ''}"
                                               placeholder="Enter password">
                                    </div>
                                </div>
                            </div>

                            <!-- Bearer Token Fields (shown when authType = bearer) -->
                            <div id="bearerTokenFields" style="display: ${config.authType === 'bearer' ? 'block' : 'none'};">
                                <div class="form-group">
                                    <label for="authToken" class="form-label">Bearer Token</label>
                                    <input type="password" id="authToken" class="form-control"
                                           value="${config.authToken || ''}"
                                           placeholder="Enter bearer token">
                                    <div class="form-hint">Token will be sent as: Authorization: Bearer {token}</div>
                                </div>
                            </div>

                            <!-- OAuth 2.0 Fields (shown when authType = oauth2) -->
                            <div id="oauth2Fields" style="display: ${config.authType === 'oauth2' ? 'block' : 'none'};">
                                <div class="form-hint" style="color: #6c757d; padding: 12px; background: #f8f9fa; border-radius: 6px;">
                                    ⚠️ OAuth 2.0 is coming soon. For now, please use Bearer Token with a pre-obtained OAuth token.
                                </div>
                            </div>
                        </div>
                    </div>
                `;
            default:
                return '<div class="config-placeholder">Select connectivity type to configure</div>';
        }
        */
    }
    /**
     * Update target configuration panel when connectivity or transformation flow changes
     */
    updateTargetConfigPanel(container) {
        const targetConnectivitySelect = container.querySelector('#targetConnectivity');
        const targetConfigPanelDiv = container.querySelector('#targetConfigPanel');

        if (!targetConnectivitySelect || !targetConfigPanelDiv) {
            console.warn('⚠️ Target config elements not found');
            return;
        }

        // Get current values
        const connectivity = targetConnectivitySelect.value;
        const wizardData = this.controller?.model?.data || {};

        console.log('🔄 Updating target config panel - connectivity:', connectivity, 'transformationFlow:', wizardData.transformationFlow);

        // Re-render panel with transformation flow passed through
        targetConfigPanelDiv.innerHTML = this.getTargetConfigPanel(connectivity, {
            ...(wizardData.targetConfig || {}),
            transformationFlow: wizardData.transformationFlow
        });

        // Re-attach event listeners for new content
        this.setupTargetConfigListeners(container);

        console.log('✅ Target config panel updated');
    }

    /**
     * Update source configuration panel when connectivity changes
     */
    updateSourceConfigPanel(container) {
        const sourceConnectivitySelect = container.querySelector('#sourceConnectivity');
        const sourceTypeSelect = container.querySelector('#sourceType');
        const sourceConfigPanelDiv = container.querySelector('#sourceConfigPanel');

        if (!sourceConnectivitySelect || !sourceConfigPanelDiv) {
            console.warn('⚠️ Source config elements not found');
            return;
        }

        // Get current values
        const connectivity = sourceConnectivitySelect.value;
        const sourceType = sourceTypeSelect?.value || '';
        const wizardData = this.controller?.model?.data || {};

        console.log('🔄 Updating source config panel - connectivity:', connectivity, 'sourceType:', sourceType);

        // Re-render panel with current config
        sourceConfigPanelDiv.innerHTML = this.getSourceConfigPanel(connectivity, wizardData.sourceConfig || {}, sourceType);

        // Re-attach event listeners for dynamic elements (like auth type dropdown)
        if (typeof InterfaceConfigComponents !== 'undefined') {
            console.log('🔧 Re-attaching shared component event listeners...');
            InterfaceConfigComponents.attachEventListeners(sourceConfigPanelDiv, '');
            console.log('✅ Event listeners re-attached');
        }

        console.log('✅ Source config panel updated');
    }

    /**
     * Setup event listeners for target configuration panel
     * Called after panel is rendered or re-rendered
     */
    setupTargetConfigListeners(container) {
        console.log('🔧 Setting up target config listeners...');

        // Target endpoint input
        const targetEndpointInput = container.querySelector('#targetEndpoint');
        if (targetEndpointInput) {
            targetEndpointInput.addEventListener('input', (e) => {
                this.dispatchEvent(new CustomEvent('fieldChange', {
                    detail: { field: 'targetEndpoint', value: e.target.value }
                }));
            });
            console.log('✅ Target endpoint input listener attached');
        }

        // Delivery mode radio buttons (for FHIR target)
        const deliveryModeBundle = container.querySelector('#deliveryModeBundle');
        const deliveryModeIndividual = container.querySelector('#deliveryModeIndividual');
        const resourceSelectionDiv = container.querySelector('#individualResourceSelection');

        if (deliveryModeBundle) {
            deliveryModeBundle.addEventListener('change', (e) => {
                if (e.target.checked) {
                    console.log('📦 Delivery mode: Bundle (single transaction)');
                    // Hide resource selection when Bundle is selected
                    if (resourceSelectionDiv) {
                        resourceSelectionDiv.style.display = 'none';
                    }
                    this.dispatchEvent(new CustomEvent('fieldChange', {
                        detail: { field: 'deliveryMode', value: 'bundle' }
                    }));
                }
            });
        }

        if (deliveryModeIndividual) {
            deliveryModeIndividual.addEventListener('change', (e) => {
                if (e.target.checked) {
                    console.log('📄 Delivery mode: Individual (separate API calls)');
                    // Show resource selection when Individual is selected
                    if (resourceSelectionDiv) {
                        resourceSelectionDiv.style.display = 'block';
                        console.log('✅ Resource selection panel shown');
                    }
                    this.dispatchEvent(new CustomEvent('fieldChange', {
                        detail: { field: 'deliveryMode', value: 'individual' }
                    }));
                }
            });
        }

        if (deliveryModeBundle || deliveryModeIndividual) {
            console.log('✅ Delivery mode radio listeners attached');
        }

        // Resource checkboxes (for Individual mode)
        const resourceCheckboxes = container.querySelectorAll('input[name="individualResources"]');
        resourceCheckboxes.forEach(checkbox => {
            checkbox.addEventListener('change', () => {
                const selectedResources = Array.from(resourceCheckboxes)
                    .filter(cb => cb.checked)
                    .map(cb => cb.value);
                console.log('📋 Selected resources:', selectedResources);
                this.dispatchEvent(new CustomEvent('fieldChange', {
                    detail: { field: 'individualResources', value: selectedResources }
                }));
            });
        });

        if (resourceCheckboxes.length > 0) {
            console.log(`✅ Resource checkbox listeners attached (${resourceCheckboxes.length} resources)`);
        }

        // Authentication toggle (for FHIR target)
        const targetAuthCheckbox = container.querySelector('#targetAuthEnabled');
        const authConfigDiv = container.querySelector('#authConfig');
        if (targetAuthCheckbox && authConfigDiv) {
            targetAuthCheckbox.addEventListener('change', (e) => {
                if (e.target.checked) {
                    authConfigDiv.style.display = 'block';
                    console.log('✅ Authentication enabled - showing auth config');
                } else {
                    authConfigDiv.style.display = 'none';
                    console.log('❌ Authentication disabled - hiding auth config');
                }

                this.dispatchEvent(new CustomEvent('fieldChange', {
                    detail: { field: 'targetAuthEnabled', value: e.target.checked }
                }));
            });
            console.log('✅ Authentication toggle listener attached');
        }

        // Authentication type selector
        const authTypeSelect = container.querySelector('#authType');
        const basicAuthFields = container.querySelector('#basicAuthFields');
        const bearerTokenFields = container.querySelector('#bearerTokenFields');
        const oauth2Fields = container.querySelector('#oauth2Fields');

        if (authTypeSelect) {
            authTypeSelect.addEventListener('change', (e) => {
                const authType = e.target.value;
                console.log('🔐 Auth type changed:', authType);

                // Hide all auth field groups
                if (basicAuthFields) basicAuthFields.style.display = 'none';
                if (bearerTokenFields) bearerTokenFields.style.display = 'none';
                if (oauth2Fields) oauth2Fields.style.display = 'none';

                // Show the selected auth field group
                if (authType === 'basic' && basicAuthFields) {
                    basicAuthFields.style.display = 'block';
                } else if (authType === 'bearer' && bearerTokenFields) {
                    bearerTokenFields.style.display = 'block';
                } else if (authType === 'oauth2' && oauth2Fields) {
                    oauth2Fields.style.display = 'block';
                }

                this.dispatchEvent(new CustomEvent('fieldChange', {
                    detail: { field: 'authType', value: authType }
                }));
            });
            console.log('✅ Auth type selector listener attached');
        }

        // Default FHIR Operation selector
        const defaultOperationSelect = container.querySelector('#defaultOperation');
        if (defaultOperationSelect) {
            defaultOperationSelect.addEventListener('change', (e) => {
                const operation = e.target.value;
                console.log('🔧 Default operation changed:', operation);
                this.dispatchEvent(new CustomEvent('fieldChange', {
                    detail: { field: 'defaultOperation', value: operation }
                }));
            });
            console.log('✅ Default operation selector listener attached');
        }

        // Auth field inputs
        const authUsernameInput = container.querySelector('#authUsername');
        const authPasswordInput = container.querySelector('#authPassword');
        const bearerTokenInput = container.querySelector('#bearerToken');
        const oauth2ClientIdInput = container.querySelector('#oauth2ClientId');
        const oauth2ClientSecretInput = container.querySelector('#oauth2ClientSecret');
        const oauth2TokenUrlInput = container.querySelector('#oauth2TokenUrl');

        if (authUsernameInput) {
            authUsernameInput.addEventListener('input', (e) => {
                this.dispatchEvent(new CustomEvent('fieldChange', {
                    detail: { field: 'authUsername', value: e.target.value }
                }));
            });
        }

        if (authPasswordInput) {
            authPasswordInput.addEventListener('input', (e) => {
                this.dispatchEvent(new CustomEvent('fieldChange', {
                    detail: { field: 'authPassword', value: e.target.value }
                }));
            });
        }

        if (bearerTokenInput) {
            bearerTokenInput.addEventListener('input', (e) => {
                this.dispatchEvent(new CustomEvent('fieldChange', {
                    detail: { field: 'bearerToken', value: e.target.value }
                }));
            });
        }

        if (oauth2ClientIdInput) {
            oauth2ClientIdInput.addEventListener('input', (e) => {
                this.dispatchEvent(new CustomEvent('fieldChange', {
                    detail: { field: 'oauth2ClientId', value: e.target.value }
                }));
            });
        }

        if (oauth2ClientSecretInput) {
            oauth2ClientSecretInput.addEventListener('input', (e) => {
                this.dispatchEvent(new CustomEvent('fieldChange', {
                    detail: { field: 'oauth2ClientSecret', value: e.target.value }
                }));
            });
        }

        if (oauth2TokenUrlInput) {
            oauth2TokenUrlInput.addEventListener('input', (e) => {
                this.dispatchEvent(new CustomEvent('fieldChange', {
                    detail: { field: 'oauth2TokenUrl', value: e.target.value }
                }));
            });
        }

        console.log('✅ Target config listeners setup complete');
    }

    /**
     * Step 4: HL7 to FHIR Transformation
     */
    getStep4Template(data) {
        console.log('🎨 [STEP 4] getStep4Template called with data:', {
            hasFhirTransformResult: !!data?.fhirTransformResult,
            mappingCount: data?.fhirTransformResult?.atomicMappings?.length || 0,
            hasAtomicMappings: !!data?.atomicMappings,
            directMappingCount: data?.atomicMappings?.length || 0,
            detectedMessageType: data?.detectedMessageType,
            dataKeys: Object.keys(data || {})
        });

        return `
            <div class="wizard-step-content" data-step="3">
                <div class="wizard-step-header">
                    <h3>🔄 ${data.transformationFlow === 'fhir_receiver' ? 'Processing Configuration' : 'HL7 to FHIR Mapping Configuration'}</h3>
                    <p>${data.transformationFlow === 'fhir_receiver' ? 'Configure processing options for received messages' : 'Review and customize field mappings between HL7 segments and FHIR resources'}</p>
                </div>

                <div class="wizard-form">
                    <!-- Mapping Configuration Interface -->
                    <div class="fhir-mapping-interface">
                        <!-- Configuration Header -->
                        <div class="mapping-config-header" style="background: linear-gradient(135deg, #f8f9ff 0%, #fff 100%); border: 2px solid #f8bbd9; border-radius: 12px; padding: 16px 20px; margin-bottom: 16px;">
                            ${(() => {
                                const hl7Ver = data?.parsedHL7Data?.version || data?.parsedHL7Data?.messageHeader?.version || null;
                                const isOldVer = hl7Ver && !['2.5', '2.5.1', '2.6', '2.7', '2.8'].includes(hl7Ver);
                                return isOldVer ? `<div style="background:#fff7ed;border:1px solid #fed7aa;border-radius:6px;padding:8px 12px;margin-bottom:10px;font-size:12px;color:#92400e;display:flex;align-items:center;gap:8px;">
                                    <span>⚠️</span>
                                    <span>Your sample uses <strong>HL7 v${hl7Ver}</strong>. Standard FHIR R4 mappings are optimized for HL7 v2.5+. Some field positions may differ — review highlighted mappings carefully.</span>
                                </div>` : '';
                            })()}
                            <div style="display: flex; justify-content: space-between; align-items: center;">
                                <div>
                                    <h4 style="margin: 0 0 6px 0; font-size: 16px; font-weight: 600; color: #1e3a8a;">🗺️ Field Mapping Configuration</h4>
                                    <div style="display: flex; align-items: center; gap: 8px; font-size: 12px; color: #6b7280;">
                                        <span style="background: #e0e7ff; color: #1e3a8a; padding: 2px 8px; border-radius: 6px; font-weight: 500;" id="fhir-message-type">${data.detectedMessageType || data.parsedHL7Data?.messageType?.name || 'Select message type above'}</span>
                                        <span style="color: #d1d5db;">•</span>
                                        <span style="background: #dcfce7; color: #166534; padding: 2px 8px; border-radius: 6px; font-weight: 500;" id="fhir-hl7-version">${data?.parsedHL7Data?.version ? 'HL7 v' + data.parsedHL7Data.version : 'HL7 v2.x'}</span>
                                        <span style="color: #d1d5db;">→</span>
                                        <select id="mapping-fhir-version-select"
                                            onchange="window._onMappingFHIRVersionChange(this.value)"
                                            style="background:#e0e7ff;color:#1e3a8a;border:1px solid #a5b4fc;border-radius:6px;font-weight:600;font-size:12px;padding:1px 6px;cursor:pointer;outline:none;">
                                            <option value="R4"   ${(!data.targetConfig?.version || data.targetConfig?.version === 'R4')   ? 'selected' : ''}>FHIR R4</option>
                                            <option value="R5"   ${data.targetConfig?.version === 'R5'   ? 'selected' : ''}>FHIR R5</option>
                                        </select>
                                        <span style="color: #d1d5db;">•</span>
                                        <span style="padding: 2px 8px; border-radius: 6px; font-weight: 500; font-size: 11px; text-transform: uppercase;" id="mapping-count-badge">
                                            <span id="mapping-count">${this.getMappingCount(data)}</span> Mappings
                                        </span>
                                    </div>
                                </div>
                                <div style="display: flex; gap: 12px;">
                                    <button style="padding: 8px 16px; border: 2px solid #f472b6; border-radius: 6px; font-size: 13px; font-weight: 500; cursor: pointer; display: flex; align-items: center; gap: 6px; background: white; color: #be185d; transition: all 0.2s ease;"
                                            id="btn-ai-suggest-mappings" onclick="window.aiSuggestMappings()">
                                        💡 AI Suggest
                                    </button>
                                    <button style="padding: 8px 16px; border: 2px solid #6366f1; border-radius: 6px; font-size: 13px; font-weight: 500; cursor: pointer; display: flex; align-items: center; gap: 6px; background: white; color: #6366f1; transition: all 0.2s ease;"
                                            id="btn-view-fhir-json" onclick="window.viewRawFHIRJSON()">
                                        📄 View FHIR JSON
                                    </button>
                                    <button style="padding: 8px 16px; border: none; border-radius: 6px; font-size: 13px; font-weight: 500; cursor: pointer; display: flex; align-items: center; gap: 6px; background: #1e3a8a; color: white; transition: all 0.2s ease;"
                                            id="btn-apply-mappings" onclick="window.applyAndContinue()">
                                        ✓ Apply &amp; Continue
                                    </button>
                                </div>
                            </div>
                        </div>

                        <!-- Mapping View Controls -->
                        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px;">
                            <div style="display: flex; gap: 8px;">
                                <button class="mapping-view-btn active" data-view="grouped" style="padding: 8px 16px; border: 2px solid #e5e7eb; border-radius: 6px; font-size: 13px; font-weight: 500; cursor: pointer; background: #1e3a8a; color: white;">
                                    📋 By Resource
                                </button>
                                <button class="mapping-view-btn" data-view="list" style="padding: 8px 16px; border: 2px solid #e5e7eb; border-radius: 6px; font-size: 13px; font-weight: 500; cursor: pointer; background: white; color: #6b7280;">
                                    📝 All Mappings
                                </button>
                                <button class="mapping-view-btn" data-view="validation" style="padding: 8px 16px; border: 2px solid #e5e7eb; border-radius: 6px; font-size: 13px; font-weight: 500; cursor: pointer; background: white; color: #6b7280;">
                                    ⚠️ Issues Only
                                </button>
                            </div>
                            <div style="display: flex; gap: 8px; align-items: center;">
                                <span style="font-size: 12px; color: #6b7280;">Filter:</span>
                                <select id="resource-filter" style="padding: 4px 8px; border: 1px solid #d1d5db; border-radius: 4px; font-size: 12px;">
                                    <option value="all">All Resources</option>
                                    ${(() => {
                                        // Build options from actual mappings when available, else from expected
                                        const mappings = data?.fhirTransformResult?.atomicMappings || [];
                                        let resourceNames = [...new Set(
                                            mappings.map(m => {
                                                const path = m.resourceType || m.targetPath || m.fhirPath || m.fhirField || '';
                                                const match = path.match(/^([A-Z][A-Za-z]+)/);
                                                return match ? match[1] : null;
                                            }).filter(Boolean)
                                        )];
                                        if (resourceNames.length === 0) {
                                            resourceNames = this.getExpectedResourcesForMessageType(data?.detectedMessageType || '').map(r => r.name);
                                        }
                                        return resourceNames.map(r => `<option value="${r}">${r}</option>`).join('');
                                    })()}
                                </select>
                            </div>
                        </div>

                        <!-- Mapping Coverage Summary -->
                        <div id="mapping-coverage-bar" style="display:flex; align-items:center; gap:16px; padding:10px 14px; background:#f0f9ff; border:1px solid #bae6fd; border-radius:8px; margin-bottom:10px; font-size:12px; color:#0369a1;">
                            ${(() => {
                                const mappings = data?.fhirTransformResult?.atomicMappings || [];
                                if (mappings.length === 0) return `<span style="color:#6b7280;">Mapping preview will appear below — select a message type</span>`;
                                const high = mappings.filter(m => m.confidence == null || m.confidence >= 0.90).length;
                                const med  = mappings.filter(m => m.confidence != null && m.confidence >= 0.75 && m.confidence < 0.90).length;
                                const low  = mappings.filter(m => m.confidence != null && m.confidence < 0.75).length;
                                const errors = data?.fhirTransformResult?.validationErrors?.length || 0;
                                return `
                                    <span style="font-weight:600;">📊 ${mappings.length} fields mapped</span>
                                    <span style="color:#d1d5db;">|</span>
                                    ${high  > 0 ? `<span style="color:#16a34a;">●&nbsp;${high} high</span>`  : ''}
                                    ${med   > 0 ? `<span style="color:#d97706;">●&nbsp;${med} medium</span>` : ''}
                                    ${low   > 0 ? `<span style="color:#dc2626;">●&nbsp;${low} low</span>`   : ''}
                                    <span style="color:#d1d5db;">|</span>
                                    <span style="${errors > 0 ? 'color:#dc2626;font-weight:600;' : 'color:#16a34a;'}">
                                        ${errors > 0 ? `⚠ ${errors} validation issue${errors > 1 ? 's' : ''}` : '✓ No validation issues'}
                                    </span>`;
                            })()}
                        </div>

                        <!-- Message Type Preview Selector -->
                        ${(() => {
                            const FAMILY_REPR = { ADT: 'ADT^A01', ORU: 'ORU^R01', ORM: 'ORM^O01', OML: 'OML^O21', SIU: 'SIU^S12', MDM: 'MDM^T02', MFN: 'MFN^M02', VXU: 'VXU^V04', DFT: 'DFT^P03', BAR: 'BAR^P01' };
                            const families = data?.acceptedMessageFamilies;
                            let previewTypes;
                            if (families && families.length > 0) {
                                previewTypes = families.map(f => f.includes('^') ? f : (FAMILY_REPR[f] || f));
                            } else {
                                previewTypes = Object.values(FAMILY_REPR);
                            }
                            const active = data?.detectedMessageType || previewTypes[0];
                            if (previewTypes.length === 0) return '';
                            const pills = previewTypes.map(t => {
                                const isActive = t === active || t.split('^')[0] === active?.split('^')[0];
                                return `<button class="msg-type-preview-btn${isActive ? ' active' : ''}"
                                    data-msg-type="${t}"
                                    onclick="window._previewMappingForType('${t}', this)"
                                    style="padding:6px 14px; border: 2px solid ${isActive ? '#1e3a8a' : '#e5e7eb'}; border-radius:20px; font-size:12px; font-weight:500; cursor:pointer; background:${isActive ? '#1e3a8a' : 'white'}; color:${isActive ? 'white' : '#6b7280'}; transition: all 0.2s;">
                                    ${t}
                                </button>`;
                            }).join('');
                            return `<div style="display:flex; align-items:center; gap:8px; flex-wrap:wrap; margin-bottom:12px; padding:10px 14px; background:#f8fafc; border:1px solid #e2e8f0; border-radius:8px;">
                                <span style="font-size:12px; color:#6b7280; white-space:nowrap; font-weight:500;">Preview mapping for:</span>
                                ${pills}
                                <span id="msg-type-preview-loading" style="display:none; font-size:12px; color:#6b7280; margin-left:4px;">⟳ Loading...</span>
                            </div>`;
                        })()}

                        <!-- Mapping Results Container -->
                        <div id="fhir-mapping-container" style="min-height: 400px; border: 2px solid #e2e8f0; border-radius: 12px; background: white;">
                            ${this.getFHIRMappingContent(data)}
                        </div>

                        <!-- Z-Segment Configuration Panel -->
                        <div id="zseg-config-panel-container">
                            ${this.getZSegmentConfigPanelHTML(data)}
                        </div>

                        <!-- Advanced Options Panel (Collapsible) -->
                        <div style="margin-top: 16px;">
                            <button id="toggle-advanced-options" style="width: 100%; padding: 12px; border: 2px solid #f3f4f6; border-radius: 8px; background: #fafafa; color: #374151; font-size: 14px; font-weight: 500; cursor: pointer; display: flex; justify-content: space-between; align-items: center;">
                                <span>🔧 Advanced Mapping Options</span>
                                <span id="advanced-toggle-icon">▼</span>
                            </button>
                            <div id="advanced-options-panel" style="display: none; margin-top: 8px; border: 2px solid #f3f4f6; border-radius: 8px; background: white; padding: 16px;">
                                <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(250px, 1fr)); gap: 20px;">
                                    <div>
                                        <label style="display: flex; align-items: center; gap: 6px; font-size: 13px; font-weight: 500; color: #374151; margin-bottom: 4px;">
                                            Data Transformation Rules
                                            <span title="Controls which HL7 fields are mapped to FHIR.&#10;&#10;• Standard — maps all commonly used fields (recommended for most integrations)&#10;• Minimal — maps only the FHIR required fields; smaller output, faster processing&#10;• Comprehensive — maps every available field including optional ones; largest output&#10;• Custom — you control every field individually in the mapping editor below"
                                                  style="cursor: help; color: #6b7280; font-size: 14px; line-height: 1;">ⓘ</span>
                                        </label>
                                        <select id="transformation-preset"
                                                onchange="window._onAdvancedOptionChange('transformationPreset', this.value)"
                                                style="width: 100%; padding: 8px; border: 1px solid #d1d5db; border-radius: 4px; font-size: 13px;">
                                            <option value="standard"       ${(data.mappingOptions?.transformationPreset || 'standard') === 'standard'       ? 'selected' : ''}>Standard HL7 → FHIR</option>
                                            <option value="minimal"        ${data.mappingOptions?.transformationPreset === 'minimal'        ? 'selected' : ''}>Minimal Required Fields</option>
                                            <option value="comprehensive"  ${data.mappingOptions?.transformationPreset === 'comprehensive'  ? 'selected' : ''}>Comprehensive Mapping</option>
                                            <option value="custom"         ${data.mappingOptions?.transformationPreset === 'custom'         ? 'selected' : ''}>Custom Rules</option>
                                        </select>
                                    </div>
                                    <div>
                                        <label style="display: flex; align-items: center; gap: 6px; font-size: 13px; font-weight: 500; color: #374151; margin-bottom: 4px;">
                                            Validation Level
                                            <span title="How strictly the generated FHIR output is checked.&#10;&#10;• Strict — every cardinality, data type and terminology binding must be correct; errors block delivery&#10;• Moderate — structural errors block delivery, terminology warnings are logged but allowed through (recommended)&#10;• Lenient — only critical structural errors are blocked; everything else is logged and passed through"
                                                  style="cursor: help; color: #6b7280; font-size: 14px; line-height: 1;">ⓘ</span>
                                        </label>
                                        <select id="validation-level"
                                                onchange="window._onAdvancedOptionChange('validationLevel', this.value)"
                                                style="width: 100%; padding: 8px; border: 1px solid #d1d5db; border-radius: 4px; font-size: 13px;">
                                            <option value="strict"   ${data.mappingOptions?.validationLevel === 'strict'   ? 'selected' : ''}>Strict FHIR Compliance</option>
                                            <option value="moderate" ${(data.mappingOptions?.validationLevel || 'moderate') === 'moderate' ? 'selected' : ''}>Moderate (Recommended)</option>
                                            <option value="lenient"  ${data.mappingOptions?.validationLevel === 'lenient'  ? 'selected' : ''}>Lenient</option>
                                        </select>
                                    </div>
                                    <div>
                                        <label style="display: flex; align-items: center; gap: 6px; font-size: 13px; font-weight: 500; color: #374151; margin-bottom: 4px;">
                                            Missing Field Handling
                                            <span title="What happens when an HL7 field referenced in a mapping is empty or absent.&#10;&#10;• Skip — the FHIR field is simply omitted (safest; FHIR receivers must tolerate absent optional fields)&#10;• Use Default Values — a system default is substituted (e.g. 'UNK' for unknown); useful when the destination requires every field&#10;• Report as Error — the message is marked failed and routed to the error queue; use when missing data must never be silently dropped"
                                                  style="cursor: help; color: #6b7280; font-size: 14px; line-height: 1;">ⓘ</span>
                                        </label>
                                        <select id="missing-field-handling"
                                                onchange="window._onAdvancedOptionChange('missingFieldHandling', this.value)"
                                                style="width: 100%; padding: 8px; border: 1px solid #d1d5db; border-radius: 4px; font-size: 13px;">
                                            <option value="skip"    ${(data.mappingOptions?.missingFieldHandling || 'skip') === 'skip'    ? 'selected' : ''}>Skip Missing Fields</option>
                                            <option value="default" ${data.mappingOptions?.missingFieldHandling === 'default' ? 'selected' : ''}>Use Default Values</option>
                                            <option value="error"   ${data.mappingOptions?.missingFieldHandling === 'error'   ? 'selected' : ''}>Report as Error</option>
                                        </select>
                                    </div>
                                </div>
                            </div>
                        </div>
                    </div>

                    <!-- Validation Messages -->
                    <div class="validation-messages" id="validationMessages">
                        ${data.validation && data.validation.errors && Object.keys(data.validation.errors).length > 0 ?
                            `<div class="validation-message error">
                                <span class="validation-icon">⚠️</span>
                                <span class="validation-text">${Object.values(data.validation.errors)[0]}</span>
                            </div>` : ''
                        }
                        ${data.validation && data.validation.warnings && Object.keys(data.validation.warnings).length > 0 ?
                            `<div class="validation-message warning">
                                <span class="validation-icon">💡</span>
                                <span class="validation-text">${Object.values(data.validation.warnings)[0]}</span>
                            </div>` : ''
                        }
                    </div>
                </div>
            </div>
        `;
    }

    /**
     * Get the current mapping count for display in Step 4 header
     */
    getMappingCount(data) {
        console.log('🔍 [DEBUG] getMappingCount called with data:', data);

        // Check multiple possible sources for mappings
        const sources = [
            { name: 'data.fhirTransformResult.atomicMappings', value: data?.fhirTransformResult?.atomicMappings },
            { name: 'data.atomicMappings', value: data?.atomicMappings },
            { name: 'data.mappings', value: data?.mappings },
            { name: 'data.fieldMappings', value: data?.fieldMappings },
            { name: 'window.wizardController.model.data.fhirTransformResult.atomicMappings', value: window.wizardController?.model?.data?.fhirTransformResult?.atomicMappings },
            { name: 'window.lastFhirResult.atomicMappings', value: window.lastFhirResult?.atomicMappings }
        ];

        console.log('🔍 [DEBUG] Checking all possible mapping sources:');
        sources.forEach(source => {
            console.log(`   - ${source.name}:`, source.value ? `${source.value.length} items` : 'undefined/null');
            if (source.value && source.value.length > 0) {
                console.log(`     Sample mapping:`, source.value[0]);
            }
        });

        // Try each source in order
        for (const source of sources) {
            if (source.value && Array.isArray(source.value) && source.value.length > 0) {
                console.log(`✅ [DEBUG] Using mapping count from ${source.name}: ${source.value.length}`);
                return source.value.length;
            }
        }

        console.log('⚠️ [DEBUG] No mappings found in any source, returning 0');
        return 0;
    }

    /**
     * Render the Z-segment configuration panel HTML for Step 3.
     * The panel is only shown when Z-segments were detected in the sample message.
     * Instantiates ZSegmentConfigPanel, stores it on the controller for later
     * access during the save flow, and returns the HTML string.
     */
    getZSegmentConfigPanelHTML(data) {
        const segmentData = data.detectedZSegmentData || [];
        if (segmentData.length === 0) return '';

        if (typeof window.ZSegmentConfigPanel !== 'function') {
            console.warn('ZSegmentConfigPanel class not loaded');
            return '';
        }

        // Create the panel and hand it to the controller so it can call
        // getConfigs() during the finish/save flow.
        const panel = new window.ZSegmentConfigPanel(segmentData);
        if (window.wizardController) {
            window.wizardController._zSegmentConfigPanel = panel;
        }

        // Events are wired in a setTimeout so the DOM is ready
        setTimeout(() => panel.attachEvents(), 0);

        return panel.render();
    }

    /**
     * FHIR Mapping Content for Step 4 - Shows current mappings and allows editing
     */
    getFHIRMappingContent(data) {
        console.log('🎨 [MAPPING CONTENT] getFHIRMappingContent called with:', {
            hasFhirTransformResult: !!data.fhirTransformResult,
            mappingsCount: data.fhirTransformResult?.atomicMappings?.length || 0,
            sampleMapping: data.fhirTransformResult?.atomicMappings?.[0]
        });

        // Check if we have FHIR transformation results with mappings
        if (data.fhirTransformResult && data.fhirTransformResult.atomicMappings && data.fhirTransformResult.atomicMappings.length > 0) {
            // Always show the mapping editor when we have mappings, regardless of validation status
            // This allows users to see and edit mappings even when there are validation errors
            console.log('✅ [MAPPING CONTENT] Rendering mapping editor with', data.fhirTransformResult.atomicMappings.length, 'mappings');
            const html = this.renderFHIRMappingEditor(data.fhirTransformResult);
            console.log('📄 [MAPPING CONTENT] Generated HTML length:', html.length);
            return html;
        } else {
            console.log('⚠️ [MAPPING CONTENT] No mappings found, showing placeholder');
            // Show placeholder for when no transformation has been attempted yet
            return this.renderFHIRMappingPlaceholder(data);
        }
    }

    /**
     * Render FHIR mapping editor showing current mappings with edit capability
     */
    renderFHIRMappingEditor(transformResult) {
        const mappings = transformResult.atomicMappings || [];
        const errors = transformResult.validationErrors || [];

        // Derive resource list from actual mapping data so we never show empty groups
        // or miss resources the backend generated (e.g. DiagnosticReport for ORU^R01).
        const resourcesFromMappings = [...new Set(
            mappings.map(m => {
                const path = m.resourceType || m.targetPath || m.fhirPath || m.fhirField || '';
                // Extract leading resource name from paths like "Patient.name[0].family"
                const match = path.match(/^([A-Z][A-Za-z]+)/);
                return match ? match[1] : null;
            }).filter(Boolean)
        )];
        // Always include expected resources for the detected message type, then add any
        // extra resources that actually appeared in the mappings.
        const messageType = window.wizardController?.model?.data?.detectedMessageType || '';
        const expectedNames = this.getExpectedResourcesForMessageType(messageType).map(r => r.name);
        const resources = [...new Set([...expectedNames, ...resourcesFromMappings])];

        // Determine status based on mappings and errors
        const hasMappings = mappings.length > 0;
        const hasErrors = errors.length > 0;
        const statusColor = hasErrors ? (hasMappings ? '#f59e0b' : '#dc2626') : '#16a34a';
        const statusBg = hasErrors ? (hasMappings ? 'linear-gradient(135deg, #fffbeb 0%, #ffffff 100%)' : 'linear-gradient(135deg, #fef2f2 0%, #ffffff 100%)') : 'linear-gradient(135deg, #f0fdf4 0%, #ffffff 100%)';
        const statusBorder = hasErrors ? (hasMappings ? '#fbbf24' : '#fecaca') : '#bbf7d0';
        const statusIcon = hasErrors ? (hasMappings ? '⚠️' : '❌') : '✅';
        const statusText = hasErrors ? (hasMappings ? 'Mappings Available - Review Validation Issues' : 'Transformation Failed') : 'Transformation Successful';

        return `
            <div style="padding: 20px;">
                <!-- Status Header -->
                <div style="background: ${statusBg}; border: 2px solid ${statusBorder}; border-radius: 12px; padding: 16px; margin-bottom: 20px;">
                    <div style="display: flex; align-items: center; gap: 12px;">
                        <span style="font-size: 20px;">${statusIcon}</span>
                        <div>
                            <h4 style="margin: 0; font-size: 16px; font-weight: 600; color: ${statusColor};">
                                ${statusText}
                            </h4>
                            <div style="font-size: 14px; color: #6b7280; margin-top: 2px;">
                                <strong>${mappings.length}</strong> field mappings generated ${hasErrors ? `| <span style="color: ${statusColor}; font-weight: 600;">${errors.length} validation issues</span>` : '| All validations passed'}
                            </div>
                            ${hasMappings && hasErrors ? `
                                <div style="font-size: 12px; color: #8b5cf6; margin-top: 4px; font-style: italic;">
                                    💡 You can edit mappings below to resolve validation issues
                                </div>
                            ` : ''}
                        </div>
                    </div>
                </div>

                <!-- Runtime resource note -->
                <div style="background:#f8fafc;border:1px solid #e2e8f0;border-radius:8px;padding:10px 14px;margin-bottom:16px;font-size:12px;color:#64748b;display:flex;align-items:flex-start;gap:8px;">
                    <span style="flex-shrink:0;margin-top:1px;">ℹ️</span>
                    <span>All resources below are based on the OOB template for this message type. At runtime, <strong>only resources whose segments are present in the incoming message</strong> will be included in the FHIR output — unused resources are automatically excluded.</span>
                </div>

                <!-- Resource Mapping Groups -->
                <div style="display: flex; flex-direction: column; gap: 16px;" id="resource-mapping-groups">
                    ${resources.map(resourceType => this.renderResourceMappingGroup(resourceType, mappings, transformResult)).join('')}
                </div>

                <!-- Validation Issues Section -->
                ${errors.length > 0 ? `
                    <div style="margin-top: 24px; background: #fef2f2; border: 2px solid #fecaca; border-radius: 12px; overflow: hidden;">
                        <div style="background: #dc2626; color: white; padding: 12px 16px;">
                            <h5 style="margin: 0; font-size: 14px; font-weight: 600;">⚠️ Validation Issues (${errors.length})</h5>
                        </div>
                        <div style="max-height: 200px; overflow-y: auto;">
                            ${errors.slice(0, 10).map((error, index) => `
                                <div style="padding: 8px 16px; border-bottom: 1px solid #fee2e2; font-size: 13px; color: #dc2626;">
                                    ${index + 1}. ${error}
                                </div>
                            `).join('')}
                            ${errors.length > 10 ? `
                                <div style="padding: 8px 16px; text-align: center; color: #6b7280; font-size: 12px; font-style: italic;">
                                    ... and ${errors.length - 10} more issues
                                </div>
                            ` : ''}
                        </div>
                    </div>
                ` : ''}
            </div>
        `;
    }

    /**
     * Render successful FHIR mapping results
     */
    renderFHIRMappingResults(transformResult) {
        const resources = transformResult.fhirResources || [];
        const mappings = transformResult.atomicMappings || [];

        return `
            <div style="padding: 20px;">
                <!-- Success Header -->
                <div style="background: linear-gradient(135deg, #f0fdf4 0%, #ffffff 100%); border: 2px solid #bbf7d0; border-radius: 12px; padding: 20px; margin-bottom: 24px;">
                    <div style="display: flex; justify-content: space-between; align-items: center;">
                        <div>
                            <div style="display: flex; align-items: center; gap: 12px; margin-bottom: 8px;">
                                <span style="font-size: 24px;">✅</span>
                                <h4 style="margin: 0; font-size: 18px; font-weight: 600; color: #16a34a;">Mapping Configuration Complete</h4>
                            </div>
                            <div style="display: flex; align-items: center; gap: 16px; color: #15803d; font-size: 14px;">
                                <span><strong>${resources.length}</strong> FHIR resources</span>
                                <span>•</span>
                                <span><strong>${mappings.length}</strong> field mappings</span>
                                <span>•</span>
                                <span>Ready for deployment</span>
                            </div>
                            ${transformResult._assemblyDriven ? `
                            <div style="margin-top:8px;font-size:12px;background:#e0e7ff;color:#3730a3;border-radius:6px;padding:6px 10px;display:inline-block;">
                                🔧 <strong>Structural Assembly</strong> — mappings are applied automatically by the HL7 assembly engine.
                                Fields marked <em>structural</em> are hardcoded; add Z-segment mappings below for custom fields.
                            </div>` : ''}
                        </div>
                        <div style="display: flex; gap: 8px;">
                            <button onclick="window.exportFHIRConfiguration()"
                                    style="padding: 8px 16px; background: #16a34a; color: white; border: none; border-radius: 6px; font-size: 12px; cursor: pointer; font-weight: 500;">
                                📤 Export Config
                            </button>
                        </div>
                    </div>
                </div>

                <!-- Generated Resources -->
                <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); gap: 16px; margin-bottom: 24px;">
                    ${resources.map((resource, index) => this.renderOptimizedFHIRResourceCard(resource, index)).join('')}
                </div>

                <!-- Applied Mappings Summary -->
                <div style="background: white; border: 2px solid #e0e7ff; border-radius: 12px; overflow: hidden;">
                    <div style="background: linear-gradient(135deg, #1e3a8a 0%, #1e40af 100%); color: white; padding: 16px;">
                        <h5 style="margin: 0; font-size: 16px; font-weight: 600;">🔗 Applied Field Mappings</h5>
                        <p style="margin: 4px 0 0 0; font-size: 14px; opacity: 0.9;">${mappings.length} HL7 fields successfully mapped to FHIR elements</p>
                    </div>
                    <div style="max-height: 300px; overflow-y: auto;">
                        ${mappings.slice(0, 15).map((mapping, index) => {
                            const isComposite = mapping._composite;
                            const isStructural = mapping._assembled && !isComposite;
                            const rowBg    = isComposite  ? '#f0fdf4' : isStructural ? '#f8faff' : '';
                            const leftBorder = isComposite ? 'border-left: 3px solid #16a34a;'
                                             : isStructural ? 'border-left: 3px solid #6366f1;'
                                             : '';
                            const badge = isComposite
                                ? `<span style="font-family:sans-serif;font-size:10px;background:#dcfce7;color:#166534;border-radius:4px;padding:1px 6px;margin-left:4px;border:1px solid #86efac;">composite</span>`
                                : isStructural
                                ? `<span style="font-family:sans-serif;font-size:10px;background:#e0e7ff;color:#3730a3;border-radius:4px;padding:1px 6px;margin-left:4px;border:1px solid #a5b4fc;">structural</span>`
                                : '';
                            const srcVal = (mapping.hl7Value || '');
                            const tgtVal = (mapping.fhirValue || mapping.transformedValue || '');
                            return `
                            <div style="padding: 12px 16px; border-bottom: 1px solid #f1f5f9; display: flex; justify-content: space-between; align-items: center;
                                        background:${rowBg}; ${leftBorder}">
                                <div style="flex: 1; font-family: monospace;">
                                    <div style="font-size: 13px; font-weight: 600; color: #1e293b;">${mapping.hl7Path || mapping.hl7Field}
                                        ${badge}
                                    </div>
                                    ${srcVal ? `<div style="font-size: 11px; color: #64748b; margin-top: 2px;">"${srcVal.substring(0, 30)}${srcVal.length > 30 ? '...' : ''}"</div>` : ''}
                                </div>
                                <div style="margin: 0 16px; color: ${isComposite ? '#16a34a' : '#6b7280'}; font-weight: bold;">→</div>
                                <div style="flex: 1; text-align: right; font-family: monospace;">
                                    <div style="font-size: 13px; font-weight: 600; color: ${isComposite ? '#166534' : '#1e40af'};">${mapping.fhirPath || mapping.fhirField}</div>
                                    ${tgtVal ? `<div style="font-size: 11px; color: #64748b; margin-top: 2px;">"${tgtVal.substring(0, 40)}${tgtVal.length > 40 ? '...' : ''}"</div>` : ''}
                                </div>
                            </div>`;
                        }).join('')}
                        ${mappings.length > 15 ? `
                            <div style="padding: 12px 16px; text-align: center; color: #6b7280; font-size: 12px; font-style: italic;">
                                ... and ${mappings.length - 15} more mappings
                            </div>
                        ` : ''}
                    </div>
                </div>
            </div>
        `;
    }

    /**
     * Render FHIR mapping placeholder when no transformation has occurred yet
     */
    renderFHIRMappingPlaceholder(data) {
        return `
            <div style="text-align: center; padding: 60px 20px;">
                <div style="font-size: 64px; margin-bottom: 24px; color: #1e3a8a;">🗺️</div>
                <div style="font-size: 20px; font-weight: 600; margin-bottom: 12px; color: #1e3a8a;">Ready to Configure Mappings</div>
                <div style="font-size: 16px; color: #6b7280; margin-bottom: 32px; max-width: 500px; margin-left: auto; margin-right: auto; line-height: 1.5;">
                    ${data.detectedMessageType ? `Configure how HL7 ${data.detectedMessageType} fields map to FHIR ${data.targetConfig?.version || 'R4'} resources` : 'Parse an HL7 message first to configure field mappings'}
                </div>

                ${data.detectedMessageType ? `
                    <button onclick="window.generateFHIRMappings()"
                            style="padding: 12px 24px; background: #1e3a8a; color: white; border: none; border-radius: 8px; font-size: 16px; font-weight: 600; cursor: pointer; margin-right: 12px;">
                        🚀 Auto-Generate Mappings
                    </button>
                    <button onclick="window.createCustomMapping()"
                            style="padding: 12px 24px; background: white; color: #1e3a8a; border: 2px solid #1e3a8a; border-radius: 8px; font-size: 16px; font-weight: 600; cursor: pointer;">
                        🔧 Create Custom Mapping
                    </button>
                ` : `
                    <div style="font-size: 14px; color: #dc2626; background: #fef2f2; border: 1px solid #fecaca; border-radius: 8px; padding: 12px; display: inline-block;">
                        ⚠️ Complete Step 2 (HL7 Parsing) first to proceed with mapping configuration
                    </div>
                `}

                <!-- Expected FHIR Resources Preview -->
                ${data.detectedMessageType ? `
                    <div style="margin-top: 48px;">
                        <div style="font-size: 16px; font-weight: 600; color: #374151; margin-bottom: 16px;">Expected FHIR Resources for ${data.detectedMessageType}:</div>
                        <div style="display: flex; justify-content: center; gap: 16px; flex-wrap: wrap;">
                            ${this.getExpectedResourcesForMessageType(data.detectedMessageType).map(resource => `
                                <div style="background: ${resource.color}; border: 1px solid ${resource.borderColor}; border-radius: 8px; padding: 12px; text-align: center; min-width: 120px;">
                                    <div style="font-size: 18px; font-weight: 600; color: ${resource.textColor};">${resource.icon}</div>
                                    <div style="font-size: 12px; color: ${resource.textColor}; margin-top: 4px; font-weight: 500;">${resource.name}</div>
                                </div>
                            `).join('')}
                        </div>
                    </div>
                ` : ''}
            </div>
        `;
    }

    /**
     * Render resource mapping group (e.g., Patient mappings, Encounter mappings)
     */
    renderResourceMappingGroup(resourceType, allMappings, transformResult) {
        const resourceMappings = allMappings.filter(mapping => {
            const targetPath = mapping.targetPath || mapping.fhirPath || mapping.fhirField || '';
            return targetPath.startsWith(resourceType + '.') || (mapping.resourceType === resourceType);
        });

        const hasIssues = transformResult.validationErrors &&
            transformResult.validationErrors.some(error => error.includes(resourceType));

        if (resourceMappings.length === 0) {
            return `
                <div style="border: 2px dashed #d1d5db; border-radius: 12px; padding: 16px; text-align: center; color: #6b7280;" id="resource-group-empty-${resourceType}">
                    <div style="font-size: 32px; margin-bottom: 8px;">${this.getResourceIcon(resourceType)}</div>
                    <div style="font-size: 14px; font-weight: 500;">No ${resourceType} mappings generated</div>
                    <div style="font-size: 12px; color: #9ca3af; margin-top: 4px;">This resource may not be present in the HL7 message</div>
                    <button onclick="window.addResourceMapping('${resourceType}')"
                            style="margin-top: 8px; padding: 6px 12px; background: #1e3a8a; color: white; border: none; border-radius: 4px; font-size: 12px; cursor: pointer;">
                        ➕ Add Mapping
                    </button>
                </div>
            `;
        }

        // Compute avg confidence across all mappings in this resource group
        const confValues = resourceMappings
            .map(m => m.confidence != null ? m.confidence : null)
            .filter(c => c !== null);
        const hasConfidence = confValues.length > 0;
        const avgConf = hasConfidence ? Math.round(confValues.reduce((a, b) => a + b, 0) / confValues.length * 100) : null;
        const highConf  = confValues.filter(c => c >= 0.90).length;
        const medConf   = confValues.filter(c => c >= 0.75 && c < 0.90).length;
        const lowConf   = confValues.filter(c => c < 0.75).length;
        const avgColor  = avgConf == null ? '#94a3b8' : avgConf >= 90 ? '#4ade80' : avgConf >= 75 ? '#fbbf24' : '#f87171';
        const barWidth  = avgConf != null ? avgConf : 0;

        const headerBg  = hasIssues ? '#fef2f2' : 'linear-gradient(135deg, #1e3a8a 0%, #1e40af 100%)';
        const headerColor = hasIssues ? '#dc2626' : 'white';

        return `
            <div class="resource-group-card" data-resource="${resourceType}" style="border: 2px solid ${hasIssues ? '#fecaca' : '#e0e7ff'}; border-radius: 12px; background: white; overflow: hidden;">
                <!-- Resource Header -->
                <div class="resource-group-header" style="background: ${headerBg}; color: ${headerColor}; padding: 12px 16px;">
                    <div style="display: flex; justify-content: space-between; align-items: flex-start;">
                        <div>
                            <h5 style="margin: 0; font-size: 14px; font-weight: 600;">${this.getResourceIcon(resourceType)} ${resourceType} Resource</h5>
                            <div style="font-size: 12px; opacity: 0.85; margin-top: 2px;">${resourceMappings.length} field mappings</div>
                        </div>
                        <div style="display: flex; align-items: center; gap: 8px;">
                            ${hasConfidence ? `
                            <!-- Confidence meter -->
                            <div style="display:flex;flex-direction:column;align-items:flex-end;gap:3px;" title="Avg mapping confidence for ${resourceType}">
                                <div style="display:flex;align-items:center;gap:6px;">
                                    <span style="font-size:10px;opacity:0.85;">confidence</span>
                                    <span style="font-size:13px;font-weight:700;color:${avgColor};">${avgConf}%</span>
                                </div>
                                <div style="width:90px;height:4px;background:rgba(255,255,255,0.25);border-radius:4px;overflow:hidden;">
                                    <div style="width:${barWidth}%;height:100%;background:${avgColor};border-radius:4px;transition:width 0.3s;"></div>
                                </div>
                                <div style="display:flex;gap:4px;font-size:9px;opacity:0.85;">
                                    ${highConf > 0  ? `<span style="color:#4ade80;">●${highConf} high</span>` : ''}
                                    ${medConf  > 0  ? `<span style="color:#fbbf24;">●${medConf} med</span>`  : ''}
                                    ${lowConf  > 0  ? `<span style="color:#f87171;">●${lowConf} low</span>`  : ''}
                                </div>
                            </div>
                            ` : ''}
                            <button onclick="window.addResourceMapping('${resourceType}')"
                                    style="padding: 4px 8px; background: rgba(255,255,255,0.2); color: inherit; border: 1px solid rgba(255,255,255,0.3); border-radius: 4px; font-size: 11px; cursor: pointer;">
                                ➕ Add Field
                            </button>
                            <button onclick="window.toggleResourceGroup('${resourceType}')"
                                    style="padding: 4px 8px; background: rgba(255,255,255,0.2); color: inherit; border: 1px solid rgba(255,255,255,0.3); border-radius: 4px; font-size: 11px; cursor: pointer;">
                                👁️ Toggle
                            </button>
                        </div>
                    </div>
                </div>

                <!-- Mappings List -->
                <div id="resource-mappings-${resourceType}" style="max-height: 400px; overflow-y: auto;">
                    ${resourceMappings.map((mapping, index) => this.renderMappingRow(mapping, index, resourceType)).join('')}
                </div>
            </div>
        `;
    }

    /**
     * Extract HL7 value from parsed data (e.g., PID.5.1 -> "Doe")
     */
    extractHL7Value(hl7Path) {
        try {
            const data = window.wizardController?.model?.data?.parsedHL7Data;
            if (!data?.basicSegments) return '';

            // Parse path like "PID.5.1" or "MSH.9"
            const parts = hl7Path.split('.');
            if (parts.length < 2) return '';

            const segment = parts[0]; // e.g., "PID"
            const field = parts[1];   // e.g., "5"
            const subfield = parts[2]; // e.g., "1" (optional)

            const segmentData = data.basicSegments[segment];
            if (!segmentData?.fields) return '';

            const fieldKey = `${segment}.${field}`;
            let value = segmentData.fields[fieldKey] || '';

            // Handle subfields (e.g., PID.5 = "Doe^John^A", subfield 1 = "Doe")
            if (subfield && value) {
                const subfields = value.split('^');
                value = subfields[parseInt(subfield) - 1] || '';
            }

            return value || '';
        } catch (error) {
            console.warn('Error extracting HL7 value:', error);
            return '';
        }
    }

    /**
     * Extract FHIR value from transformed resources (e.g., Patient.name[0].family -> "Doe")
     */
    extractFHIRValue(fhirPath, resourceType) {
        try {
            const data = window.wizardController?.model?.data?.fhirTransformResult;
            if (!data?.fhirResources) return '';

            // Find the resource of the correct type
            const resource = data.fhirResources.find(r => r.resourceType === resourceType);
            if (!resource) return '';

            // Simple path extraction (supports basic paths like "Patient.birthDate" or "Patient.name[0].family")
            const cleanPath = fhirPath.replace(resourceType + '.', '');

            // Navigate the path
            const parts = cleanPath.split('.');
            let value = resource;

            for (const part of parts) {
                if (!value) break;

                // Handle array notation like "name[0]"
                const arrayMatch = part.match(/^(.+?)\[(\d+)\]$/);
                if (arrayMatch) {
                    const key = arrayMatch[1];
                    const index = parseInt(arrayMatch[2]);
                    value = value[key]?.[index];
                } else {
                    value = value[part];
                }
            }

            return value || '';
        } catch (error) {
            console.warn('Error extracting FHIR value:', error);
            return '';
        }
    }

    /**
     * Syntax highlight JSON for display
     */
    syntaxHighlightJSON(obj) {
        let json = JSON.stringify(obj, null, 2);

        // Escape HTML
        json = json.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

        // Syntax highlighting with colors
        json = json.replace(/("(\\u[a-zA-Z0-9]{4}|\\[^u]|[^\\"])*"(\s*:)?|\b(true|false|null)\b|-?\d+(?:\.\d*)?(?:[eE][+\-]?\d+)?)/g, function (match) {
            let cls = 'color: #ce9178;'; // string - orange
            if (/^"/.test(match)) {
                if (/:$/.test(match)) {
                    cls = 'color: #9cdcfe;'; // key - light blue
                }
            } else if (/true|false/.test(match)) {
                cls = 'color: #569cd6;'; // boolean - blue
            } else if (/null/.test(match)) {
                cls = 'color: #569cd6;'; // null - blue
            } else {
                cls = 'color: #b5cea8;'; // number - light green
            }
            return '<span style="' + cls + '">' + match + '</span>';
        });

        return json;
    }

    /**
     * Render individual mapping row with edit capabilities
     */
    renderMappingRow(mapping, index, resourceType) {
        const hasIssue = mapping.error || mapping.warning;
        const hl7Field = mapping.sourcePath || mapping.hl7Path || mapping.hl7Field || 'Unknown';
        const fhirField = mapping.targetPath || mapping.fhirPath || mapping.fhirField || 'Unknown';
        const transformType = mapping.transformType || mapping.transform || 'direct';

        // Confidence — only show when a meaningful value is present (treat 0 as absent)
        const hasConf = mapping.confidence != null && mapping.confidence > 0;
        const conf = hasConf ? mapping.confidence : null;
        const confPct = conf != null ? Math.round(conf * 100) : null;
        const confColor = confPct == null ? '#94a3b8' : confPct >= 90 ? '#16a34a' : confPct >= 75 ? '#d97706' : '#dc2626';
        const confLabel = confPct != null
            ? `<span class="confidence-badge"
                     style="font-size:11px;font-weight:700;color:${confColor};background:${confColor}18;padding:2px 7px;border-radius:10px;border:1px solid ${confColor}50;white-space:nowrap;"
                     title="Mapping confidence (spec-driven)">${confPct}% conf.</span>`
            : '';

        // Extract actual values from parsed HL7 data and FHIR resources
        const _hl7Raw = this.extractHL7Value(hl7Field);
        const _fhirRaw = this.extractFHIRValue(fhirField, resourceType);
        const hl7Value  = (_hl7Raw  == null) ? '' : (typeof _hl7Raw  === 'string' ? _hl7Raw  : JSON.stringify(_hl7Raw));
        const fhirValue = (_fhirRaw == null) ? '' : (typeof _fhirRaw === 'string' ? _fhirRaw : JSON.stringify(_fhirRaw));

        return `
            <div class="fhir-mapping-row" style="padding: 10px 16px; border-bottom: 1px solid #f1f5f9; display: flex; align-items: center; gap: 12px; ${hasIssue ? 'background: #fef2f2;' : ''}"
                 data-mapping-index="${index}" data-resource="${resourceType}">

                <!-- HL7 Source -->
                <div style="flex: 1; min-width: 0;">
                    <div style="display: flex; align-items: center; gap: 6px; margin-bottom: 3px;">
                        <input type="text" value="${hl7Field}"
                               onchange="window.updateMapping(${index}, 'hl7Field', this.value)"
                               style="flex: 1; padding: 4px 8px; border: 1px solid #d1d5db; border-radius: 4px; font-size: 12px; font-family: monospace; background: #f9fafb;"
                               readonly>
                        ${hasIssue ? '<span style="color: #dc2626; font-size: 12px;" title="Validation issue">⚠️</span>' : ''}
                    </div>
                    <div style="font-size: 11px; color: #6b7280; padding: 2px 4px; background: #f3f4f6; border-radius: 4px; font-family: monospace; max-width: 200px; overflow: hidden; text-overflow: ellipsis;">
                        "${hl7Value.substring(0, 25)}${hl7Value.length > 25 ? '...' : ''}"
                    </div>
                </div>

                <!-- Transformation Arrow & Type Badge -->
                <div style="display: flex; flex-direction: column; align-items: center; gap: 4px; flex-shrink: 0;">
                    <div style="color: #6b7280; font-weight: bold; font-size: 14px;">→</div>
                    ${window._transformBadge(transformType, index)}
                </div>

                <!-- FHIR Target -->
                <div style="flex: 1; min-width: 0;">
                    <div style="margin-bottom: 3px;">
                        <select onchange="window.updateMapping(${index}, 'fhirField', this.value)"
                                style="width: 100%; padding: 4px 8px; border: 1px solid #d1d5db; border-radius: 4px; font-size: 12px; font-family: monospace;">
                            <option value="${fhirField}" selected>${fhirField}</option>
                            ${this.getFHIRFieldOptions(resourceType).map(option =>
                                option.value !== fhirField ? `<option value="${option.value}">${option.label}</option>` : ''
                            ).join('')}
                        </select>
                    </div>
                    <div style="font-size: 11px; color: #6b7280; padding: 2px 4px; background: #f0f9ff; border-radius: 4px; font-family: monospace; max-width: 200px; overflow: hidden; text-overflow: ellipsis;">
                        "${fhirValue.substring(0, 25)}${fhirValue.length > 25 ? '...' : ''}"
                    </div>
                </div>

                <!-- Confidence + Actions -->
                <div style="display: flex; flex-direction: column; align-items: flex-end; gap: 5px; flex-shrink: 0;">
                    ${confLabel}
                    <div style="display: flex; gap: 4px;">
                        <button onclick="window.editMappingDetails(${index})"
                                style="padding: 3px 7px; background: #f3f4f6; color: #374151; border: 1px solid #d1d5db; border-radius: 4px; font-size: 11px; cursor: pointer;" title="Edit Details">
                            ✏️
                        </button>
                        <button onclick="window.deleteMappingRow(${index})"
                                style="padding: 3px 7px; background: #fef2f2; color: #dc2626; border: 1px solid #fecaca; border-radius: 4px; font-size: 11px; cursor: pointer;" title="Delete">
                            🗑️
                        </button>
                    </div>
                </div>
            </div>
        `;
    }

    /**
     * Get expected FHIR resources for a message type
     */
    getExpectedResourcesForMessageType(messageType) {
        // Strip variant suffix for matching (ADT^A08 → ADT, SIU^S12 → SIU, etc.)
        const baseType = (messageType || '').split('^')[0].toUpperCase();

        const resourceMap = {
            // ADT — all variants produce Patient + Encounter
            'ADT^A01': [
                { name: 'Patient',       icon: '👤', color: '#f0fdf4', borderColor: '#bbf7d0', textColor: '#15803d' },
                { name: 'Encounter',     icon: '🏥', color: '#eff6ff', borderColor: '#bfdbfe', textColor: '#1d4ed8' },
                { name: 'MessageHeader', icon: '📨', color: '#fdf2f8', borderColor: '#fbcfe8', textColor: '#be185d' }
            ],
            // ORU — result messages
            'ORU^R01': [
                { name: 'MessageHeader',    icon: '📨', color: '#fdf4ff', borderColor: '#e9d5ff', textColor: '#7c3aed' },
                { name: 'Patient',          icon: '👤', color: '#f0fdf4', borderColor: '#bbf7d0', textColor: '#15803d' },
                { name: 'DiagnosticReport', icon: '📊', color: '#fff7ed', borderColor: '#fed7aa', textColor: '#c2410c' },
                { name: 'Observation',      icon: '🔬', color: '#fdf2f8', borderColor: '#fbcfe8', textColor: '#be185d' }
            ],
        };

        // Fallback by base message type (covers all variants)
        const baseMap = {
            'ADT': [
                { name: 'Patient',       icon: '👤', color: '#f0fdf4', borderColor: '#bbf7d0', textColor: '#15803d' },
                { name: 'Encounter',     icon: '🏥', color: '#eff6ff', borderColor: '#bfdbfe', textColor: '#1d4ed8' },
                { name: 'MessageHeader', icon: '📨', color: '#fdf2f8', borderColor: '#fbcfe8', textColor: '#be185d' }
            ],
            'ORU': [
                { name: 'Patient',          icon: '👤', color: '#f0fdf4', borderColor: '#bbf7d0', textColor: '#15803d' },
                { name: 'DiagnosticReport', icon: '📊', color: '#fff7ed', borderColor: '#fed7aa', textColor: '#c2410c' },
                { name: 'Observation',      icon: '🔬', color: '#fdf2f8', borderColor: '#fbcfe8', textColor: '#be185d' }
            ],
            'ORM': [
                { name: 'Patient',        icon: '👤', color: '#f0fdf4', borderColor: '#bbf7d0', textColor: '#15803d' },
                { name: 'ServiceRequest', icon: '📋', color: '#eff6ff', borderColor: '#bfdbfe', textColor: '#1d4ed8' }
            ],
            'OML': [
                { name: 'Patient',          icon: '👤', color: '#f0fdf4', borderColor: '#bbf7d0', textColor: '#15803d' },
                { name: 'ServiceRequest',   icon: '📋', color: '#eff6ff', borderColor: '#bfdbfe', textColor: '#1d4ed8' },
                { name: 'DiagnosticReport', icon: '📊', color: '#fff7ed', borderColor: '#fed7aa', textColor: '#c2410c' }
            ],
            'MFN': [
                { name: 'Organization',  icon: '🏢', color: '#f0fdf4', borderColor: '#bbf7d0', textColor: '#15803d' },
                { name: 'Practitioner',  icon: '👨‍⚕️', color: '#eff6ff', borderColor: '#bfdbfe', textColor: '#1d4ed8' },
                { name: 'Location',      icon: '📍', color: '#fff7ed', borderColor: '#fed7aa', textColor: '#c2410c' }
            ],
            'SIU': [
                { name: 'Appointment', icon: '📅', color: '#f0fdf4', borderColor: '#bbf7d0', textColor: '#15803d' },
                { name: 'Patient',     icon: '👤', color: '#eff6ff', borderColor: '#bfdbfe', textColor: '#1d4ed8' },
                { name: 'Practitioner',icon: '👨‍⚕️', color: '#fff7ed', borderColor: '#fed7aa', textColor: '#c2410c' }
            ],
            'DFT': [
                { name: 'ChargeItem', icon: '💰', color: '#f0fdf4', borderColor: '#bbf7d0', textColor: '#15803d' },
                { name: 'Patient',    icon: '👤', color: '#eff6ff', borderColor: '#bfdbfe', textColor: '#1d4ed8' },
                { name: 'Procedure',  icon: '⚕️',  color: '#fff7ed', borderColor: '#fed7aa', textColor: '#c2410c' }
            ],
            'MDM': [
                { name: 'DocumentReference', icon: '📄', color: '#f0fdf4', borderColor: '#bbf7d0', textColor: '#15803d' },
                { name: 'Patient',           icon: '👤', color: '#eff6ff', borderColor: '#bfdbfe', textColor: '#1d4ed8' }
            ],
            'VXU': [
                { name: 'Immunization', icon: '💉', color: '#f0fdf4', borderColor: '#bbf7d0', textColor: '#15803d' },
                { name: 'Patient',      icon: '👤', color: '#eff6ff', borderColor: '#bfdbfe', textColor: '#1d4ed8' }
            ],
            'BAR': [
                { name: 'Account',   icon: '🏦', color: '#f0fdf4', borderColor: '#bbf7d0', textColor: '#15803d' },
                { name: 'Patient',   icon: '👤', color: '#eff6ff', borderColor: '#bfdbfe', textColor: '#1d4ed8' },
                { name: 'Encounter', icon: '🏥', color: '#fff7ed', borderColor: '#fed7aa', textColor: '#c2410c' }
            ],
            'RDS': [
                { name: 'MedicationDispense', icon: '💊', color: '#f0fdf4', borderColor: '#bbf7d0', textColor: '#15803d' },
                { name: 'Patient',            icon: '👤', color: '#eff6ff', borderColor: '#bfdbfe', textColor: '#1d4ed8' }
            ],
        };

        return resourceMap[messageType] || baseMap[baseType] || [
            { name: 'Patient',       icon: '👤', color: '#f0fdf4', borderColor: '#bbf7d0', textColor: '#15803d' },
            { name: 'MessageHeader', icon: '📨', color: '#fdf2f8', borderColor: '#fbcfe8', textColor: '#be185d' }
        ];
    }

    /**
     * Get icon for FHIR resource type
     */
    getResourceIcon(resourceType) {
        const icons = {
            'Patient': '👤',
            'Encounter': '🏥',
            'MessageHeader': '📨',
            'Observation': '🔬',
            'DiagnosticReport': '📊',
            'Practitioner': '👨‍⚕️',
            'Organization': '🏢'
        };
        return icons[resourceType] || '📋';
    }

    /**
     * Get FHIR field options for a resource type (simplified for now)
     */
    getFHIRFieldOptions(resourceType) {
        const fieldMap = {
            'Patient': [
                { value: 'Patient.identifier[0].value', label: 'Patient ID' },
                { value: 'Patient.name[0].family', label: 'Family Name' },
                { value: 'Patient.name[0].given[0]', label: 'Given Name' },
                { value: 'Patient.birthDate', label: 'Birth Date' },
                { value: 'Patient.gender', label: 'Gender' },
                { value: 'Patient.address[0].line[0]', label: 'Address Line' },
                { value: 'Patient.telecom[0].value', label: 'Phone/Email' }
            ],
            'Encounter': [
                { value: 'Encounter.status', label: 'Status' },
                { value: 'Encounter.class.code', label: 'Class Code' },
                { value: 'Encounter.subject.reference', label: 'Patient Reference' },
                { value: 'Encounter.period.start', label: 'Start Date' },
                { value: 'Encounter.location[0].location.display', label: 'Location' }
            ],
            'MessageHeader': [
                { value: 'MessageHeader.id', label: 'Message ID' },
                { value: 'MessageHeader.source.name', label: 'Source System' },
                { value: 'MessageHeader.eventCoding.code', label: 'Event Code' },
                { value: 'MessageHeader.timestamp', label: 'Timestamp' }
            ]
        };
        return fieldMap[resourceType] || [];
    }

    /**
     * FHIR Placeholder Content for Step 4
     */
    getFHIRPlaceholderContent(data) {
        if (data.fhirTransformResult) {
            // Show FHIR transformation results
            return this.getFHIRTransformationResults(data.fhirTransformResult);
        } else {
            // Show placeholder
            return `
                <div style="text-align: center; padding: 60px 20px;">
                    <div style="font-size: 64px; margin-bottom: 24px; color: #1e3a8a;">🔄</div>
                    <div style="font-size: 20px; font-weight: 600; margin-bottom: 12px; color: #1e3a8a;">Ready for FHIR Transformation</div>
                    <div style="font-size: 16px; color: #6b7280; margin-bottom: 32px; max-width: 400px; margin-left: auto; margin-right: auto; line-height: 1.5;">
                        Click "Transform to FHIR" to convert your parsed HL7 data into FHIR R4 resources
                    </div>
                    <div style="display: flex; justify-content: center; gap: 16px; flex-wrap: wrap;">
                        <div style="background: #f0fdf4; border: 1px solid #bbf7d0; border-radius: 8px; padding: 12px; text-align: center; min-width: 120px;">
                            <div style="font-size: 18px; font-weight: 600; color: #16a34a;">👤</div>
                            <div style="font-size: 12px; color: #15803d; margin-top: 4px;">Patient</div>
                        </div>
                        <div style="background: #eff6ff; border: 1px solid #bfdbfe; border-radius: 8px; padding: 12px; text-align: center; min-width: 120px;">
                            <div style="font-size: 18px; font-weight: 600; color: #2563eb;">🏥</div>
                            <div style="font-size: 12px; color: #1d4ed8; margin-top: 4px;">Encounter</div>
                        </div>
                        <div style="background: #fdf2f8; border: 1px solid #fbcfe8; border-radius: 8px; padding: 12px; text-align: center; min-width: 120px;">
                            <div style="font-size: 18px; font-weight: 600; color: #db2777;">🔬</div>
                            <div style="font-size: 12px; color: #be185d; margin-top: 4px;">Observation</div>
                        </div>
                    </div>
                </div>
            `;
        }
    }

    /**
     * FHIR Transformation Results Display
     */
    getFHIRTransformationResults(fhirResult) {
        const resources = fhirResult.fhirResources || [];
        const atomicMappings = fhirResult.atomicMappings || [];

        return `
            <div style="padding: 20px;">
                <!-- Summary Header -->
                <div style="background: linear-gradient(135deg, #f0fdf4 0%, #ffffff 100%); border: 2px solid #bbf7d0; border-radius: 12px; padding: 20px; margin-bottom: 24px;">
                    <div style="display: flex; justify-content: space-between; align-items: center;">
                        <div>
                            <div style="display: flex; align-items: center; gap: 12px; margin-bottom: 8px;">
                                <span style="font-size: 24px;">✅</span>
                                <h4 style="margin: 0; font-size: 18px; font-weight: 600; color: #16a34a;">Transformation Complete</h4>
                            </div>
                            <div style="display: flex; align-items: center; gap: 16px; color: #15803d; font-size: 14px;">
                                <span><strong>${resources.length}</strong> FHIR resources created</span>
                                <span>•</span>
                                <span><strong>${atomicMappings.length}</strong> field mappings applied</span>
                                <span>•</span>
                                <span>FHIR ${data.targetConfig?.version || 'R4'} compliant</span>
                            </div>
                        </div>
                        <div style="display: flex; gap: 8px;">
                            <button onclick="window.viewOptimizedFHIRJSON()"
                                    style="padding: 8px 16px; background: #16a34a; color: white; border: none; border-radius: 6px; font-size: 12px; cursor: pointer; font-weight: 500;">
                                📄 View JSON
                            </button>
                        </div>
                    </div>
                </div>

                <!-- FHIR Resources Grid -->
                <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); gap: 16px; margin-bottom: 24px;">
                    ${resources.map((resource, index) => this.renderOptimizedFHIRResourceCard(resource, index)).join('')}
                </div>

                <!-- Field Mappings by Resource -->
                ${atomicMappings.length > 0 ? (() => {
                    const resourceTypes = [...new Set(
                        [...resources.map(r => r.resourceType), ...atomicMappings.map(m => {
                            const path = m.resourceType || m.targetPath || m.fhirPath || m.fhirField || '';
                            const match = path.match(/^([A-Z][A-Za-z]+)/);
                            return match ? match[1] : null;
                        })].filter(Boolean)
                    )];
                    return `
                    <div style="margin-top: 8px;">
                        <div style="font-size: 13px; font-weight: 600; color: #374151; margin-bottom: 12px; display: flex; align-items: center; gap: 8px;">
                            🔗 Field Mappings by Resource
                            <span style="font-size: 11px; font-weight: 400; color: #6b7280;">(${atomicMappings.length} total)</span>
                        </div>
                        <div style="display: flex; flex-direction: column; gap: 12px;">
                            ${resourceTypes.map(rt => this.renderResourceMappingGroup(rt, atomicMappings, fhirResult)).join('')}
                        </div>
                    </div>`;
                })() : ''}
            </div>
        `;
    }

    /**
     * Render FHIR resource card for optimized wizard
     */
    renderOptimizedFHIRResourceCard(resource, index) {
        const resourceType = resource.resourceType;
        const icon = this.getFHIRResourceIcon(resourceType);
        const summary = this.getFHIRResourceSummary(resource);

        return `
            <div style="background: white; border: 2px solid #e2e8f0; border-radius: 12px; overflow: hidden; transition: all 0.3s ease;">
                <div style="background: linear-gradient(135deg, #1e3a8a 0%, #1e40af 100%); color: white; padding: 16px;">
                    <div style="display: flex; justify-content: space-between; align-items: center;">
                        <div style="display: flex; align-items: center; gap: 12px;">
                            <span style="font-size: 24px;">${icon}</span>
                            <div>
                                <h5 style="margin: 0; font-size: 16px; font-weight: 600;">${resourceType}</h5>
                                <p style="margin: 4px 0 0 0; font-size: 12px; opacity: 0.9;">Resource ${index + 1}</p>
                            </div>
                        </div>
                        <button onclick="window.viewOptimizedResourceDetails(${index})"
                                style="background: rgba(255,255,255,0.2); border: 1px solid rgba(255,255,255,0.3); color: white; border-radius: 6px; padding: 6px 12px; font-size: 11px; cursor: pointer;">
                            View Details
                        </button>
                    </div>
                </div>
                <div style="padding: 16px;">
                    ${summary}
                </div>
            </div>
        `;
    }

    /**
     * Get FHIR resource icon
     */
    getFHIRResourceIcon(resourceType) {
        const icons = {
            'Patient': '👤',
            'MessageHeader': '📨',
            'Encounter': '🏥',
            'Observation': '🔬',
            'DiagnosticReport': '📋',
            'Procedure': '💉',
            'Organization': '🏢',
            'Practitioner': '👨‍⚕️',
            'Bundle': '📦'
        };
        return icons[resourceType] || '📄';
    }

    /**
     * Get FHIR resource summary
     */
    getFHIRResourceSummary(resource) {
        switch (resource.resourceType) {
            case 'Patient':
                const name = resource.name?.[0];
                const nameStr = name ? `${name.given?.[0] || ''} ${name.family || ''}`.trim() : 'N/A';
                return `
                    <div style="space-y: 8px;">
                        <div style="margin-bottom: 8px;"><strong>Name:</strong> ${nameStr}</div>
                        <div style="margin-bottom: 8px;"><strong>Gender:</strong> ${resource.gender || 'N/A'}</div>
                        <div style="margin-bottom: 8px;"><strong>Birth Date:</strong> ${resource.birthDate || 'N/A'}</div>
                        <div style="margin-bottom: 8px;"><strong>ID:</strong> ${resource.id || 'N/A'}</div>
                    </div>
                `;
            case 'Encounter':
                return `
                    <div style="space-y: 8px;">
                        <div style="margin-bottom: 8px;"><strong>Status:</strong> ${resource.status || 'N/A'}</div>
                        <div style="margin-bottom: 8px;"><strong>Class:</strong> ${resource.class?.code || 'N/A'}</div>
                        <div style="margin-bottom: 8px;"><strong>Period:</strong> ${resource.period?.start || 'N/A'}</div>
                        <div style="margin-bottom: 8px;"><strong>ID:</strong> ${resource.id || 'N/A'}</div>
                    </div>
                `;
            case 'Observation':
                return `
                    <div style="space-y: 8px;">
                        <div style="margin-bottom: 8px;"><strong>Status:</strong> ${resource.status || 'N/A'}</div>
                        <div style="margin-bottom: 8px;"><strong>Code:</strong> ${resource.code?.coding?.[0]?.display || resource.code?.coding?.[0]?.code || 'N/A'}</div>
                        <div style="margin-bottom: 8px;"><strong>Value:</strong> ${resource.valueQuantity?.value || resource.valueString || 'N/A'}</div>
                        <div style="margin-bottom: 8px;"><strong>ID:</strong> ${resource.id || 'N/A'}</div>
                    </div>
                `;
            default:
                return `
                    <div style="color: #6b7280; font-style: italic;">
                        ${resource.resourceType} resource with ${Object.keys(resource).length} properties
                    </div>
                `;
        }
    }

    /**
     * Step 5: Mapping Configuration (Most Complex)
     */
    getStep5Template(data) {
        return `
            <div class="wizard-step-content" data-step="5">
                <div class="wizard-step-header">
                    <h3>${data.transformationFlow === 'fhir_receiver' ? 'Message Mapping' : 'HL7 to FHIR Mapping'}</h3>
                    <p>${data.transformationFlow === 'fhir_receiver' ? 'Configure how received messages are processed and stored' : 'Configure how HL7 messages are transformed to FHIR resources'}</p>
                </div>

                <div class="wizard-form">
                    <!-- Message Type Selection -->
                    <div class="form-group">
                        <label for="messageType" class="form-label required">HL7 Message Type</label>
                        <select id="messageType" class="form-control" required>
                            <option value="ADT^A01" ${data.messageType === 'ADT^A01' ? 'selected' : ''}>ADT^A01 - Admit Patient</option>
                            <option value="ADT^A03" ${data.messageType === 'ADT^A03' ? 'selected' : ''}>ADT^A03 - Discharge Patient</option>
                            <option value="ORU^R01" ${data.messageType === 'ORU^R01' ? 'selected' : ''}>ORU^R01 - Lab Results</option>
                            <option value="ORM^O01" ${data.messageType === 'ORM^O01' ? 'selected' : ''}>ORM^O01 - Order Message</option>
                            <option value="custom" ${data.messageType === 'custom' ? 'selected' : ''}>Custom Message Type</option>
                        </select>
                    </div>

                    <!-- OOB Template Selection -->
                    <div class="oob-mapping-templates">
                        <h4>🚀 Out-of-Box Mapping Templates</h4>
                        <div class="oob-template-list">
                            ${this.getOOBMappingTemplates(data.availableTemplates, data.mappingTemplate)}
                        </div>
                    </div>

                    <!-- Custom Mapping Interface -->
                    <div class="mapping-interface" id="mappingInterface">
                        <div class="mapping-tabs">
                            <button type="button" class="mapping-tab active" data-tab="visual">
                                <span class="tab-icon">🎨</span>
                                Visual Mapping
                            </button>
                            <button type="button" class="mapping-tab" data-tab="table">
                                <span class="tab-icon">📊</span>
                                Table View
                            </button>
                            <button type="button" class="mapping-tab" data-tab="preview">
                                <span class="tab-icon">👁️</span>
                                Preview
                            </button>
                        </div>

                        <div class="mapping-content">
                            <div class="mapping-panel active" id="visualMapping">
                                ${this.getVisualMappingPanel(data.customMappings)}
                            </div>
                            <div class="mapping-panel" id="tableMapping">
                                ${this.getTableMappingPanel(data.customMappings)}
                            </div>
                            <div class="mapping-panel" id="previewMapping">
                                ${this.getPreviewMappingPanel()}
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        `;
    }

    /**
     * Generate transformation results template - shows parsed HL7 segments first
     */
    getTransformationResultsTemplate(data) {
        if (!data.parsedHL7Data?.enhancedSegments && !data.parsedHL7Data?.segmentGroups) {
            return '<div style="text-align: center; padding: 40px; color: #6b7280;">No parsed data available</div>';
        }

        // Initialize segment viewer properties if not exists
        if (!this.expandedSegments) {
            this.expandedSegments = new Set(['MSH', 'PID']); // Smart defaults
        }
        if (!this.expandedFields) {
            this.expandedFields = new Set();
        }
        if (!this.viewMode) {
            this.viewMode = 'compact'; // compact, detailed, table
        }

        const segments = this.getSegmentsFromGroups(data.parsedHL7Data);
        const validationErrors = data.parsedHL7Data.validationErrors || [];

        // Build field metadata cache from actual API data
        this.buildFieldMetadataCache(segments);

        return `
            <div class="transformation-results-container">
                <!-- HL7 Segment Viewer -->
                <div class="compact-segment-viewer">
                    ${this.renderCompactHeader(validationErrors, data.detectedMessageType, segments, data.parsedHL7Data)}
                    ${this.renderSegmentTable(segments, validationErrors)}
                </div>

            </div>
        `;
    }

    /**
     * Render compact header with dynamic metrics (from backup design)
     */
    renderCompactHeader(validationErrors, messageType, segments, data) {
        const errorCount = validationErrors.filter(err => err.severity === 'ERROR' || err.severity === 'error').length;
        const warningCount = validationErrors.filter(err => err.severity === 'WARNING' || err.severity === 'warning').length;
        const totalFields = segments.reduce((sum, [_, seg]) => sum + (seg.fieldCount || 0), 0);

        // Dynamic segment expansion check
        const availableSegmentNames = segments.map(([segName]) => segName);
        const importantSegments = availableSegmentNames.slice(0, 3); // First 3 as important
        const hasExpandedImportant = importantSegments.some(seg => this.expandedSegments.has(seg));

        // Dynamic schema information
        const schemaInfo = this.getSchemaInfo(data);

        return `
            <div class="segment-header-compact">
                <div class="message-summary">
                    <div class="message-info">
                        <span class="message-type">${messageType?.name || messageType || 'Unknown'}</span>
                        <span class="message-desc">${messageType?.description || 'HL7 Message Successfully Parsed'}</span>
                        ${schemaInfo.html}
                    </div>
                    <div class="metrics-row">
                        <span class="metric">${segments.length} segments</span>
                        <span class="metric">${totalFields} fields</span>
                        ${errorCount > 0 ? `<span class="metric error">${errorCount} errors</span>` : ''}
                        ${warningCount > 0 ? `<span class="metric warning">${warningCount} warnings</span>` : ''}
                        <span class="metric info">v${data.version || '2.5'}</span>
                    </div>
                </div>
                <div class="view-controls">
                    <button class="view-btn ${this.viewMode === 'compact' ? 'active' : ''}" onclick="window.wizardView.setViewMode('compact')">Compact</button>
                    <button class="view-btn ${this.viewMode === 'table' ? 'active' : ''}" onclick="window.wizardView.setViewMode('table')">Table</button>
                    <button class="expand-all-btn" onclick="window.wizardView.toggleAllSegments()">
                        ${hasExpandedImportant ? 'Collapse Key' : 'Expand Key'}
                    </button>
                </div>
            </div>
        `;
    }

    /**
     * Render segments in the selected view mode (from backup design)
     */
    renderSegmentTable(segments, validationErrors) {
        if (this.viewMode === 'table') {
            return this.renderTableView(segments, validationErrors);
        }
        return this.renderCompactView(segments, validationErrors);
    }

    /**
     * Compact view with minimal vertical space usage (from backup design)
     */
    renderCompactView(segments, validationErrors) {
        return `
            <div class="segments-compact">
                ${segments.map(([segName, segment]) =>
                    this.renderCompactSegment(segName, segment, validationErrors)
                ).join('')}
            </div>
        `;
    }

    /**
     * Table view showing all segments in a grid
     */
    renderTableView(segments, validationErrors) {
        return `
            <div class="segments-table-container">
                <table class="segments-table">
                    <thead>
                        <tr>
                            <th>Segment</th>
                            <th>Description</th>
                            <th>Fields</th>
                            <th>Status</th>
                            <th>Key Values</th>
                            <th>Actions</th>
                        </tr>
                    </thead>
                    <tbody>
                        ${segments.map(([segName, segment]) =>
                            this.renderSegmentTableRow(segName, segment, validationErrors)
                        ).join('')}
                    </tbody>
                </table>
            </div>
        `;
    }

    /**
     * Get segments in correct message order using API data
     */
    /**
     * ✅ FIXED: Returns segment entries from segmentGroups (preserves all repeated instances).
     * Falls back to getSegmentsInMessageOrder when segmentGroups is unavailable.
     */
    getSegmentsFromGroups(parsedHL7Data) {
        const segmentGroups = parsedHL7Data?.segmentGroups || {};
        const segmentOrder = parsedHL7Data?.segmentOrder || [];
        const enhancedSegments = parsedHL7Data?.enhancedSegments || {};

        if (segmentOrder.length > 0 && Object.keys(segmentGroups).length > 0) {
            const segmentEntries = [];
            const instanceCounts = {};

            for (const segName of segmentOrder) {
                const idx = instanceCounts[segName] || 0;
                instanceCounts[segName] = idx + 1;

                const group = segmentGroups[segName];
                if (group && group[idx]) {
                    // First instance uses plain name so default expandedSegments still work
                    const instanceKey = idx === 0 ? segName : `${segName}[${idx}]`;
                    segmentEntries.push([instanceKey, group[idx]]);
                } else if (idx === 0 && enhancedSegments[segName]) {
                    segmentEntries.push([segName, enhancedSegments[segName]]);
                }
            }

            // Append any segments not listed in segmentOrder
            Object.keys(enhancedSegments).forEach(segName => {
                if (!segmentOrder.includes(segName)) {
                    segmentEntries.push([segName, enhancedSegments[segName]]);
                }
            });

            return segmentEntries;
        }

        // Fallback to original flat-map ordering
        return this.getSegmentsInMessageOrder(enhancedSegments, parsedHL7Data);
    }

    getSegmentsInMessageOrder(segments, parsedHL7Data) {
        // Use segment order from API if available
        if (parsedHL7Data?.segmentOrder) {
            const apiOrder = parsedHL7Data.segmentOrder;
            const segmentEntries = [];

            // First, add segments in API-specified order
            apiOrder.forEach(segName => {
                if (segments[segName]) {
                    segmentEntries.push([segName, segments[segName]]);
                }
            });

            // Then add any remaining segments not in the order
            Object.entries(segments).forEach(([segName, segment]) => {
                if (!apiOrder.includes(segName)) {
                    segmentEntries.push([segName, segment]);
                }
            });

            return segmentEntries;
        }

        // Fallback: Use sequence from segments if available
        const segmentEntries = Object.entries(segments);

        segmentEntries.sort(([nameA, segA], [nameB, segB]) => {
            // Sort by sequence if available
            const seqA = segA.sequence !== undefined ? segA.sequence : 999;
            const seqB = segB.sequence !== undefined ? segB.sequence : 999;

            if (seqA !== seqB) {
                return seqA - seqB;
            }

            // Fallback: standard HL7 segment order
            const standardOrder = ['MSH', 'EVN', 'PID', 'PD1', 'ROL', 'NK1', 'PV1', 'PV2', 'ROL', 'DB1', 'OBX', 'AL1', 'DG1', 'DRG', 'PR1', 'GT1', 'IN1', 'IN2', 'IN3', 'ACC', 'UB1', 'UB2'];
            const orderA = standardOrder.indexOf(nameA);
            const orderB = standardOrder.indexOf(nameB);

            if (orderA !== -1 && orderB !== -1) {
                return orderA - orderB;
            } else if (orderA !== -1) {
                return -1;
            } else if (orderB !== -1) {
                return 1;
            }

            // Final fallback: alphabetical order
            return nameA.localeCompare(nameB);
        });

        return segmentEntries;
    }

    /**
     * Render compact segment card (from backup design)
     */
    renderCompactSegment(segName, segment, validationErrors) {
        // segName may be an instance key like "IN1[1]" for repeated segments.
        // baseSeg is the plain HL7 name used for display and validation lookups.
        const baseSeg = segName.replace(/\[\d+\]$/, '');
        const isExpanded = this.expandedSegments.has(segName);
        const segmentErrors = validationErrors.filter(err => err.segment === baseSeg);
        const hasIssues = segmentErrors.length > 0;

        // Get specific missing required fields for this segment
        const missingRequiredFields = segmentErrors.filter(err =>
            err.code === 'MISSING_REQUIRED_FIELD' || err.code === 'EMPTY_REQUIRED_FIELD'
        );

        const keyFields = this.getKeyFields(baseSeg, segment);

        return `
            <div class="segment-compact ${hasIssues ? 'has-issues' : ''} ${isExpanded ? 'expanded' : ''} ${missingRequiredFields.length > 0 ? 'has-missing-required' : ''}">
                <div class="segment-row" onclick="toggleSegment('${segName}')">
                    <div class="segment-info">
                        <div class="segment-name-badge">
                            <span class="seg-name">${baseSeg}</span>
                            ${this.renderDynamicBadges(baseSeg, segment, segmentErrors)}
                            ${this.renderRequiredFieldsBadge(missingRequiredFields)}
                        </div>
                        <div class="segment-summary">
                            <span class="seg-desc">${this.truncateText(segment.description || `${baseSeg} Segment`, 40)}</span>
                            ${keyFields.length > 0 ? `<span class="key-values">${keyFields.join(' • ')}</span>` : ''}
                        </div>
                    </div>
                    <div class="segment-meta">
                        <span class="field-count">${segment.fieldCount || segment.fields?.length || 0}</span>
                        <span class="expand-icon ${isExpanded ? 'expanded' : ''}">${isExpanded ? '−' : '+'}</span>
                    </div>
                </div>

                ${isExpanded ? this.renderSegmentFields(baseSeg, segment, validationErrors) : ''}
                ${hasIssues && !isExpanded ? this.renderInlineValidation(segmentErrors, missingRequiredFields) : ''}
            </div>
        `;
    }

    /**
     * Render segment fields with structured table view and toggle
     */
    renderSegmentFields(segName, segment, validationErrors) {
        if (!segment.fields || segment.fields.length === 0) {
            return `<div class="no-fields">No field data available</div>`;
        }

        // Ensure fields are sorted by position, not array order
        const fields = this.sortFieldsByPosition(segment.fields);
        const fieldErrors = validationErrors.filter(err => err.segment === segName);

        // Count fields with/without values for toggle
        const fieldsWithValues = fields.filter(field => field.hasValue && field.value && field.value.trim() !== '');
        const fieldsWithoutValues = fields.filter(field => !field.hasValue || !field.value || field.value.trim() === '');

        // Dynamic schema information
        const schemaSource = segment.dictionarySource || 'Unknown';
        const isSchemaEnhanced = schemaSource.includes('Schema') || schemaSource.includes('RealSchemaLoader');

        return `
            <div class="fields-compact">
                <div class="fields-header">
                    <div class="fields-title-section">
                        <span class="fields-title fields-count">🚀 ENHANCED Fields (${fields.length})</span>
                        <span class="schema-info">
                            ${isSchemaEnhanced ?
                                '📋 HL7 Schema Definitions' :
                                '💡 Basic Descriptions'
                            }
                        </span>
                    </div>
                    <div class="field-filter-controls">
                        <div class="field-filter-info">
                            <span>📊 Fields: </span>
                            <span class="field-count-badge">${fieldsWithValues.length} with values</span>
                            <span class="field-count-badge">${fieldsWithoutValues.length} empty</span>
                        </div>
                        <div class="field-filter-toggle" onclick="toggleFieldVisibility(this, '${segName}')">
                            <span>Show all fields</span>
                            <div class="field-filter-switch" data-active="false" data-segment="${segName}"></div>
                        </div>
                    </div>
                </div>
                ${this.renderStructuredFieldsTable(segName, fields, fieldErrors)}
            </div>
        `;
    }

    /**
     * Render structured fields table with navy blue styling and atomic breakdown
     */
    renderStructuredFieldsTable(segName, fields, validationErrors = []) {
        if (!fields || fields.length === 0) {
            return '<div style="text-align: center; color: #6b7280; padding: 20px;">No fields found</div>';
        }

        // Create a clean, professional table with exact column widths
        return `
            <div class="modern-hl7-table" style="margin: 16px 0; background: white; border-radius: 12px; box-shadow: 0 4px 12px rgba(0,0,0,0.08); overflow: hidden; border: 1px solid #e2e8f0;">
                <table style="width: 100%; border-collapse: collapse; font-family: 'Inter', -apple-system, BlinkMacSystemFont, sans-serif;">
                    <colgroup>
                        <col style="width: 80px;">
                        <col style="width: 200px;">
                        <col style="width: 70px;">
                        <col style="width: 400px;">
                        <col style="width: 250px;">
                    </colgroup>
                    <thead>
                        <tr style="background: linear-gradient(135deg, #1e3a8a 0%, #3b82f6 100%);">
                            <th style="padding: 16px 12px; color: white; font-weight: 600; font-size: 12px; text-align: center; border: none;">POS</th>
                            <th style="padding: 16px 12px; color: white; font-weight: 600; font-size: 12px; text-align: left; border: none;">FIELD NAME</th>
                            <th style="padding: 16px 12px; color: white; font-weight: 600; font-size: 12px; text-align: center; border: none;">TYPE</th>
                            <th style="padding: 16px 12px; color: white; font-weight: 600; font-size: 12px; text-align: left; border: none;">VALUE</th>
                            <th style="padding: 16px 12px; color: white; font-weight: 600; font-size: 12px; text-align: left; border: none;">DESCRIPTION</th>
                        </tr>
                    </thead>
                    <tbody>
                        ${this.renderTableRows(segName, fields, validationErrors)}
                    </tbody>
                </table>
            </div>
        `;
    }

    renderTableRows(segName, fields, validationErrors) {
        return fields.map((field, idx) => {
            const hasValue = field.hasValue && field.value && field.value.trim() !== '';
            const isRequired = field.optionality === 'R';
            const valueStr = String(field.value || '');
            const hasSubcomponents = valueStr.includes('^') || valueStr.includes('~') || valueStr.includes('&');

            // Check for validation errors
            const fieldErrors = validationErrors.filter(err =>
                err.segment === segName &&
                (err.field === field.position || err.field === (idx + 1))
            );
            const hasValidationError = fieldErrors.length > 0;

            // Get field metadata with enhanced schema validation
            const fieldDefinition = this.getFieldDefinition(segName, field.position || (idx + 1));
            const fieldName = fieldDefinition?.name || field.name || `Field ${field.position || idx + 1}`;
            const fieldDescription = fieldDefinition?.description || field.description || '';

            // Enhanced data type detection with schema validation
            const schemaDataType = fieldDefinition?.dataType || fieldDefinition?.type;
            const messageDataType = field.dataType || field.type;
            const dataType = messageDataType || schemaDataType || 'ST';

            // Check for schema compliance
            const isSchemaCompliant = this.validateFieldAgainstSchema(field, fieldDefinition, segName);
            const schemaViolations = this.getSchemaViolations(field, fieldDefinition, segName);

            const fieldId = `field-${segName}-${field.position || idx + 1}`;

            // Enhanced row styling for different types of issues
            let rowStyle = '';
            let badgeColor = '#1e3a8a'; // Default blue

            if (hasValidationError) {
                rowStyle = 'background: #fef2f2; border-left: 4px solid #ef4444;';
                badgeColor = '#ef4444'; // Red for validation errors
            } else if (!isSchemaCompliant) {
                rowStyle = 'background: #fefbf2; border-left: 4px solid #f59e0b;';
                badgeColor = '#f59e0b'; // Orange for schema violations
            }

            return `
                <tr style="border-bottom: 1px solid #f1f5f9; ${rowStyle}" data-field-id="${fieldId}">
                    <td style="padding: 16px 12px; text-align: center; border: none;">
                        <div style="display: inline-flex; flex-direction: column; align-items: center; gap: 6px;">
                            <div style="background: ${badgeColor}; color: white; padding: 8px 12px; border-radius: 8px; font-weight: 700; font-size: 13px; min-width: 36px; text-align: center; box-shadow: 0 2px 4px rgba(0,0,0,0.1);">
                                ${field.position || idx + 1}
                            </div>
                            <div style="display: flex; flex-direction: column; align-items: center; gap: 2px;">
                                ${hasValue ?
                                    '<div style="width: 8px; height: 8px; background: #10b981; border-radius: 50%; box-shadow: 0 0 0 2px rgba(16, 185, 129, 0.2);"></div>' :
                                    '<div style="width: 8px; height: 8px; background: #d1d5db; border-radius: 50%;"></div>'
                                }
                                ${isRequired ? '<span style="color: #f59e0b; font-size: 9px; font-weight: 600;">REQ</span>' : ''}
                            </div>
                        </div>
                    </td>
                    <td style="padding: 16px 12px; border: none;">
                        <div style="display: flex; align-items: flex-start; gap: 10px;">
                            ${hasSubcomponents ?
                                `<button onclick="toggleFieldExpansion('${fieldId}')"
                                         style="background: none; border: none; color: #1e3a8a; font-size: 16px; cursor: pointer; padding: 4px; margin-top: 2px; border-radius: 4px; transition: all 0.2s; outline: none;"
                                         onmouseover="this.style.background='#eff6ff'" onmouseout="this.style.background='none'">
                                    <span id="expand-icon-${fieldId}">▶</span>
                                 </button>` :
                                '<span style="width: 20px;"></span>'
                            }
                            <div style="flex: 1;">
                                <div style="font-size: 14px; color: #1e293b; font-weight: 600; line-height: 1.4; margin-bottom: 2px;">
                                    ${fieldName}
                                    ${schemaDataType && messageDataType && schemaDataType !== messageDataType ?
                                        `<span style="background: #fef3c7; color: #d97706; padding: 2px 6px; border-radius: 4px; font-size: 9px; margin-left: 8px;">TYPE MISMATCH</span>` : ''
                                    }
                                </div>
                                ${hasValidationError ?
                                    `<div style="color: #ef4444; font-size: 11px; font-weight: 500; margin-top: 4px; padding: 4px 8px; background: #fee2e2; border-radius: 4px; border-left: 3px solid #ef4444;">
                                        ⚠ ${fieldErrors.map(err => err.message || err.code).join(', ')}
                                     </div>` : ''
                                }
                                ${!isSchemaCompliant && schemaViolations.length > 0 ?
                                    `<div style="color: #d97706; font-size: 11px; font-weight: 500; margin-top: 4px; padding: 4px 8px; background: #fef3c7; border-radius: 4px; border-left: 3px solid #f59e0b;">
                                        🚨 Schema: ${schemaViolations.join(', ')}
                                     </div>` : ''
                                }
                            </div>
                        </div>
                    </td>
                    <td style="padding: 16px 12px; text-align: center; border: none;">
                        <span style="background: #475569; color: white; padding: 6px 10px; border-radius: 6px; font-size: 11px; font-weight: 600; letter-spacing: 0.5px;">
                            ${dataType}
                        </span>
                    </td>
                    <td style="padding: 16px 12px; border: none;">
                        <div id="value-${fieldId}">
                            ${this.renderCleanFieldValue(field)}
                        </div>
                        <div id="expanded-${fieldId}" style="display: none; margin-top: 12px; padding: 12px; background: #f8fafc; border-radius: 8px; border-left: 4px solid #1e3a8a;">
                            ${hasSubcomponents ? this.renderExpandedSubcomponents(field) : ''}
                        </div>
                    </td>
                    <td style="padding: 16px 12px; border: none;">
                        <div style="font-size: 12px; color: #64748b; line-height: 1.5;">
                            ${fieldDescription}
                        </div>
                    </td>
                </tr>
            `;
        }).join('');
    }

    renderCleanFieldValue(field) {
        const value = field.value;
        const hasValue = value !== null && value !== undefined && String(value).trim() !== '';

        if (!hasValue) {
            return '<div style="color: #94a3b8; font-style: italic; font-size: 12px; padding: 8px; background: #f8fafc; border-radius: 6px; border: 1px dashed #cbd5e1;">No value</div>';
        }

        const valueStr = String(value);

        // Special handling for encoding characters
        if (field.name && field.name.toLowerCase().includes('encoding') && valueStr.length <= 4) {
            return this.renderEncodingCharacters(valueStr);
        }

        // Handle complex values with separators
        if (valueStr.includes('^') || valueStr.includes('~') || valueStr.includes('&')) {
            return this.renderComplexValue(valueStr, field);
        }

        // Simple value
        return `
            <div style="background: #f8fafc; border-radius: 6px; padding: 10px; border-left: 4px solid #1e3a8a;">
                <div style="font-family: 'JetBrains Mono', 'Fira Code', Consolas, monospace; font-size: 13px; color: #1e293b; word-break: break-all; line-height: 1.4;">
                    ${this.escapeHtml(valueStr)}
                </div>
                <div style="margin-top: 6px; font-size: 10px; color: #64748b; font-family: monospace; display: flex; align-items: center; gap: 8px;">
                    <span>${valueStr.length} chars</span>
                    ${(field.tableId && window.hasHL7TableData && window.hasHL7TableData(field.tableId)) ? `<span class="table-id-badge clickable-table-badge" style="font-family: 'Courier New', monospace; font-size: 9px; font-weight: 600; color: white; background: #ec4899; padding: 2px 5px; border-radius: 3px; cursor: pointer; transition: all 0.2s;" onclick="window.showHL7Table('${field.tableId}', '${field.name || 'Field'}', this)" onmouseover="this.style.background='#be185d'; this.style.transform='scale(1.05)'" onmouseout="this.style.background='#ec4899'; this.style.transform='scale(1)'">T${field.tableId}</span>` : ''}
                </div>
            </div>
        `;
    }

    renderEncodingCharacters(valueStr) {
        const meanings = {
            '^': 'Component Separator',
            '~': 'Repetition Separator',
            '\\': 'Escape Character',
            '&': 'Subcomponent Separator'
        };

        return `
            <div style="background: #fef2f2; border-radius: 6px; padding: 12px; border-left: 4px solid #ef4444;">
                <div style="font-weight: 600; color: #dc2626; margin-bottom: 8px; font-size: 12px;">⚙️ HL7 Encoding Characters</div>
                ${Array.from(valueStr).map(char => `
                    <div style="display: flex; align-items: center; gap: 8px; margin: 4px 0; padding: 6px; background: white; border-radius: 4px;">
                        <span style="background: #dc2626; color: white; padding: 4px 8px; border-radius: 4px; font-family: monospace; font-weight: bold; font-size: 12px;">${char}</span>
                        <span style="color: #374151; font-size: 11px;">${meanings[char] || 'Special Character'}</span>
                    </div>
                `).join('')}
            </div>
        `;
    }

    renderComplexValue(valueStr, field = {}) {
        return `
            <div style="background: #eff6ff; border-radius: 6px; padding: 10px; border-left: 4px solid #3b82f6;">
                <div style="font-family: 'JetBrains Mono', 'Fira Code', Consolas, monospace; font-size: 13px; color: #1e293b; word-break: break-all; line-height: 1.4; margin-bottom: 6px;">
                    ${this.escapeHtml(valueStr)}
                </div>
                <div style="font-size: 10px; color: #3b82f6; font-weight: 500; display: flex; align-items: center; gap: 8px;">
                    <span>🔗 Complex field - click ▶ to expand components</span>
                    ${(field.tableId && window.hasHL7TableData && window.hasHL7TableData(field.tableId)) ? `<span class="table-id-badge clickable-table-badge" style="font-family: 'Courier New', monospace; font-size: 9px; font-weight: 600; color: white; background: #ec4899; padding: 2px 5px; border-radius: 3px; cursor: pointer; transition: all 0.2s;" onclick="window.showHL7Table('${field.tableId}', '${field.name || 'Field'}', this)" onmouseover="this.style.background='#be185d'; this.style.transform='scale(1.05)'" onmouseout="this.style.background='#ec4899'; this.style.transform='scale(1)'">T${field.tableId}</span>` : ''}
                </div>
            </div>
        `;
    }

    /**
     * Validate field against HL7 schema
     */
    validateFieldAgainstSchema(field, fieldDefinition, segName) {
        if (!fieldDefinition) return true; // Can't validate without schema

        const value = field.value;
        if (!value) return true; // Empty fields are generally allowed

        const valueStr = String(value);

        // Check data type compliance
        if (fieldDefinition.dataType) {
            const expectedType = fieldDefinition.dataType.toUpperCase();

            switch (expectedType) {
                case 'NM': // Numeric
                    if (!/^[\d\-\+\.]+$/.test(valueStr)) return false;
                    break;
                case 'DT': // Date
                    if (!/^\d{8}$/.test(valueStr.replace(/[\-\/]/g, ''))) return false;
                    break;
                case 'TM': // Time
                    if (!/^\d{2,6}$/.test(valueStr.replace(/[:]/g, ''))) return false;
                    break;
                case 'TS': // Timestamp
                    if (!/^\d{8,14}/.test(valueStr.replace(/[\-\/:\s]/g, ''))) return false;
                    break;
                case 'ID': // Coded ID
                    if (fieldDefinition.maxLength && valueStr.length > fieldDefinition.maxLength) return false;
                    break;
            }
        }

        // Check maximum length
        if (fieldDefinition.maxLength && valueStr.length > fieldDefinition.maxLength) {
            return false;
        }

        // Check minimum length
        if (fieldDefinition.minLength && valueStr.length < fieldDefinition.minLength) {
            return false;
        }

        return true;
    }

    /**
     * Get specific schema violations for a field
     */
    getSchemaViolations(field, fieldDefinition, segName) {
        const violations = [];
        if (!fieldDefinition) return violations;

        const value = field.value;
        if (!value) return violations;

        const valueStr = String(value);

        // Check data type violations
        if (fieldDefinition.dataType) {
            const expectedType = fieldDefinition.dataType.toUpperCase();

            switch (expectedType) {
                case 'NM':
                    if (!/^[\d\-\+\.]+$/.test(valueStr)) violations.push('Invalid numeric format');
                    break;
                case 'DT':
                    if (!/^\d{8}$/.test(valueStr.replace(/[\-\/]/g, ''))) violations.push('Invalid date format (YYYYMMDD)');
                    break;
                case 'TM':
                    if (!/^\d{2,6}$/.test(valueStr.replace(/[:]/g, ''))) violations.push('Invalid time format (HHMMSS)');
                    break;
                case 'TS':
                    if (!/^\d{8,14}/.test(valueStr.replace(/[\-\/:\s]/g, ''))) violations.push('Invalid timestamp format');
                    break;
            }
        }

        // Check length violations
        if (fieldDefinition.maxLength && valueStr.length > fieldDefinition.maxLength) {
            violations.push(`Exceeds max length (${fieldDefinition.maxLength})`);
        }

        if (fieldDefinition.minLength && valueStr.length < fieldDefinition.minLength) {
            violations.push(`Below min length (${fieldDefinition.minLength})`);
        }

        return violations;
    }

    /**
     * Render atomic field value with component breakdown (^, ~, &)
     */
    renderAtomicFieldValue(field) {
        console.log('🔧 renderAtomicFieldValue called with field:', field);

        // More flexible value checking - accept any non-null, non-undefined value
        const value = field.value;
        const hasValue = value !== null && value !== undefined && String(value).trim() !== '';

        if (!hasValue) {
            return '<div style="font-size: 11px; color: #9ca3af; font-style: italic; padding: 6px; background: #f9fafb; border-radius: 4px; border: 1px dashed #d1d5db;">No value</div>';
        }

        const valueStr = String(value);
        let html = '';

        // Special handling for HL7 encoding characters field (typically "^~\&")
        if (field.name && field.name.toLowerCase().includes('encoding') && valueStr.length <= 4) {
            console.log('🎯 ENCODING FIELD DETECTED:', field.name, valueStr);
            html += '<div style="font-size: 11px; color: #dc2626; background: #fef2f2; padding: 8px; border-radius: 4px; border-left: 3px solid #dc2626;">';
            html += '<div style="font-weight: 600; margin-bottom: 6px; color: #dc2626;">⚙️ HL7 Encoding Characters:</div>';

            for (let i = 0; i < valueStr.length; i++) {
                const char = valueStr[i];
                let meaning = '';
                switch (char) {
                    case '^': meaning = 'Component Separator'; break;
                    case '~': meaning = 'Repetition Separator'; break;
                    case '\\': meaning = 'Escape Character'; break;
                    case '&': meaning = 'Subcomponent Separator'; break;
                    default: meaning = 'Special Character';
                }

                html += `<div style="margin-left: 8px; margin-bottom: 3px; font-family: 'Courier New', monospace;">
                    <span style="background: #dc2626; color: white; padding: 2px 6px; border-radius: 3px; font-size: 10px; margin-right: 8px; font-weight: bold;">${char}</span>
                    <span style="color: #374151; font-size: 10px;">${meaning}</span>
                </div>`;
            }
            html += '</div>';
        }
        // Handle complex parsing with nested separators
        else if (valueStr.includes('^') || valueStr.includes('~') || valueStr.includes('&')) {
            html += this.renderComplexHL7Value(valueStr);
        }
        // Simple value display
        else {
            console.log('🔵 SIMPLE VALUE FIELD:', field.name, valueStr);
            html += `<div style="font-size: 11px; color: #374151; font-family: 'Courier New', monospace; background: #f0f9ff; padding: 8px; border-radius: 4px; border-left: 4px solid #1e3a8a; word-break: break-all; line-height: 1.4; max-width: 100%; overflow: hidden;">
                <div style="font-weight: 600; color: #1e3a8a; margin-bottom: 4px; font-size: 10px;">🚀 ENHANCED VALUE:</div>
                <div style="word-wrap: break-word; overflow-wrap: break-word;">
                    ${this.escapeHtml(valueStr)}
                </div>
            </div>`;
        }

        // Add value metadata
        html += `<div style="margin-top: 6px; padding: 4px 6px; background: #f3f4f6; border-radius: 3px; font-size: 9px; color: #6b7280; font-family: monospace;">
            📏 Length: ${valueStr.length} chars | 🔤 Type: ${typeof value}
        </div>`;

        return html;
    }

    /**
     * Render expanded subcomponents view for complex fields
     */
    renderExpandedSubcomponents(field) {
        const valueStr = String(field.value || '');
        if (!valueStr) return '';

        let html = '<div style="font-size: 12px; font-weight: 600; color: #1e3a8a; margin-bottom: 8px;">📋 Component Breakdown:</div>';

        // Handle repetitions first
        if (valueStr.includes('~')) {
            const repetitions = valueStr.split('~');
            html += '<div style="margin-bottom: 12px;">';
            html += '<div style="font-size: 11px; font-weight: 600; color: #059669; margin-bottom: 4px;">🔄 Repetitions:</div>';
            repetitions.forEach((rep, idx) => {
                if (rep.trim()) {
                    html += `<div style="margin: 4px 0; padding: 6px; background: white; border-radius: 4px; border-left: 3px solid #059669;">
                        <div style="font-size: 10px; color: #059669; font-weight: 600;">Repetition ${idx + 1}:</div>
                        ${this.renderComponentBreakdown(rep)}
                    </div>`;
                }
            });
            html += '</div>';
        } else {
            html += this.renderComponentBreakdown(valueStr);
        }

        return html;
    }

    /**
     * Render component breakdown for a single value
     */
    renderComponentBreakdown(value) {
        if (!value.includes('^') && !value.includes('&')) {
            return `<div style="font-family: monospace; padding: 4px; background: #f9fafb; border-radius: 4px;">${this.escapeHtml(value)}</div>`;
        }

        let html = '';

        // Handle components
        if (value.includes('^')) {
            const components = value.split('^');
            html += '<div style="margin: 4px 0;">';
            html += '<div style="font-size: 10px; font-weight: 600; color: #1e3a8a; margin-bottom: 4px;">📋 Components:</div>';
            components.forEach((component, idx) => {
                // Add data type information for components
                const componentType = this.getComponentDataType(idx + 1); // Component positions start at 1
                html += `<div style="margin: 2px 0; padding: 4px 8px; background: #eff6ff; border-radius: 3px; font-size: 11px;">
                    <span style="color: #1e3a8a; font-weight: 600;">${idx + 1}:</span>
                    <span class="component-datatype-badge" style="font-family: 'Courier New', monospace; font-size: 9px; font-weight: 600; color: white; background: #1e40af; padding: 1px 4px; border-radius: 3px; margin-left: 4px; margin-right: 4px;">${componentType}</span>
                    ${component.includes('&') ? this.renderSubcomponentBreakdown(component) : `<span style="font-family: monospace;">${this.escapeHtml(component)}</span>`}
                </div>`;
            });
            html += '</div>';
        }

        return html;
    }

    /**
     * Get data type for a specific component position
     */
    getComponentDataType(componentPosition) {
        // For now, return common HL7 component types
        // This could be enhanced with real schema lookup
        const commonComponentTypes = {
            1: 'ST', // Most first components are strings
            2: 'ST',
            3: 'ID', // Often ID codes
            4: 'ST',
            5: 'ST'
        };
        return commonComponentTypes[componentPosition] || 'ST';
    }

    /**
     * Get data type for a specific subcomponent position
     */
    getSubcomponentDataType(subcomponentPosition) {
        // Common HL7 subcomponent types
        const commonSubcomponentTypes = {
            1: 'ST', // Most subcomponents are strings
            2: 'ST',
            3: 'ID',
            4: 'ST'
        };
        return commonSubcomponentTypes[subcomponentPosition] || 'ST';
    }

    /**
     * Render subcomponent breakdown
     */
    renderSubcomponentBreakdown(value) {
        if (!value.includes('&')) {
            return `<span style="font-family: monospace;">${this.escapeHtml(value)}</span>`;
        }

        const subcomponents = value.split('&');
        return subcomponents.map((sub, idx) => {
            const subcomponentType = this.getSubcomponentDataType(idx + 1);
            return `<div style="margin-left: 12px; font-size: 10px;">
                <span style="color: #7c3aed; font-weight: 600;">${idx + 1}:</span>
                <span class="subcomponent-datatype-badge" style="font-family: 'Courier New', monospace; font-size: 8px; font-weight: 600; color: white; background: #ec4899; padding: 1px 3px; border-radius: 2px; margin-left: 3px; margin-right: 3px;">${subcomponentType}</span>
                <span style="font-family: monospace;">${this.escapeHtml(sub)}</span>
            </div>`;
        }).join('');
    }

    /**
     * Toggle field expansion to show/hide subcomponents
     */
    toggleFieldExpansion(fieldId) {
        const expandedDiv = document.getElementById(`expanded-${fieldId}`);
        const iconSpan = document.getElementById(`expand-icon-${fieldId}`);

        if (!expandedDiv || !iconSpan) {
            console.warn('Could not find elements for field:', fieldId);
            return;
        }

        const isExpanded = expandedDiv.style.display !== 'none';

        if (isExpanded) {
            expandedDiv.style.display = 'none';
            iconSpan.textContent = '▶';
            iconSpan.style.transform = 'rotate(0deg)';
        } else {
            expandedDiv.style.display = 'block';
            iconSpan.textContent = '▼';
            iconSpan.style.transform = 'rotate(90deg)';
        }
    }

    /**
     * Render complex HL7 value with nested separators
     */
    renderComplexHL7Value(value) {
        let html = '';

        // First, handle repetitions (~) at the top level
        if (value.includes('~')) {
            const repetitions = value.split('~');
            html += '<div style="font-size: 11px; color: #059669; background: #ecfdf5; padding: 8px; border-radius: 4px; border-left: 3px solid #059669; margin-bottom: 4px;">';
            html += '<div style="font-weight: 600; margin-bottom: 6px; color: #059669;">🔄 REPETITIONS (~):</div>';

            repetitions.forEach((rep, idx) => {
                if (rep.trim() || idx === 0) {
                    html += `<div style="margin-left: 8px; margin-bottom: 6px; background: white; padding: 6px; border-radius: 3px; border: 1px solid #a7f3d0;">
                        <div style="font-weight: 600; color: #059669; margin-bottom: 3px; font-size: 10px;">REPETITION ${idx + 1}:</div>
                        ${this.renderComponentLevel(rep)}
                    </div>`;
                }
            });
            html += '</div>';
        }
        // Handle components (^) at the top level
        else if (value.includes('^')) {
            html += this.renderComponentLevel(value);
        }
        // Handle subcomponents (&) at the top level
        else if (value.includes('&')) {
            html += this.renderSubcomponentLevel(value);
        }

        return html;
    }

    /**
     * Render component level (^)
     */
    renderComponentLevel(value) {
        if (!value.includes('^')) {
            return this.renderSubcomponentLevel(value);
        }

        const components = value.split('^');
        let html = '<div style="font-size: 11px; color: #1e3a8a; background: #eff6ff; padding: 8px; border-radius: 4px; border-left: 3px solid #1e3a8a;">';
        html += '<div style="font-weight: 600; margin-bottom: 6px; color: #1e3a8a;">📋 COMPONENTS (^):</div>';

        components.forEach((component, idx) => {
            html += `<div style="margin-left: 8px; margin-bottom: 4px; background: white; padding: 6px; border-radius: 3px; border: 1px solid #bfdbfe;">
                <div style="font-weight: 600; color: #1e3a8a; margin-bottom: 3px; font-size: 10px;">COMPONENT ${idx + 1}:</div>`;

            if (component.trim()) {
                html += this.renderSubcomponentLevel(component);
            } else {
                html += '<em style="color: #9ca3af; font-size: 10px;">empty component</em>';
            }
            html += '</div>';
        });
        html += '</div>';

        return html;
    }

    /**
     * Render subcomponent level (&)
     */
    renderSubcomponentLevel(value) {
        if (!value.includes('&')) {
            return `<div style="font-family: 'Courier New', monospace; color: #374151; font-size: 11px; padding: 4px; background: #f9fafb; border-radius: 2px;">
                ${this.escapeHtml(value) || '<em style="color: #9ca3af;">empty</em>'}
            </div>`;
        }

        const subcomponents = value.split('&');
        let html = '<div style="font-size: 11px; color: #7c3aed; background: #f3e8ff; padding: 6px; border-radius: 4px; border-left: 3px solid #7c3aed;">';
        html += '<div style="font-weight: 600; margin-bottom: 4px; color: #7c3aed; font-size: 10px;">🧩 SUBCOMPONENTS (&):</div>';

        subcomponents.forEach((sub, idx) => {
            if (sub.trim() || idx === 0) {
                html += `<div style="margin-left: 8px; margin-bottom: 2px; font-family: 'Courier New', monospace; background: white; padding: 3px 6px; border-radius: 2px; border: 1px solid #d8b4fe;">
                    <span style="background: #7c3aed; color: white; padding: 1px 4px; border-radius: 2px; font-size: 9px; margin-right: 6px; font-weight: bold;">${idx + 1}</span>
                    <span style="color: #374151;">${this.escapeHtml(sub) || '<em style="color: #9ca3af;">empty</em>'}</span>
                </div>`;
            }
        });
        html += '</div>';

        return html;
    }

    /**
     * Toggle field visibility between populated fields only and all fields
     */
    toggleFieldVisibility(element, segName) {
        const switchElement = element.querySelector('.field-filter-switch');
        const isActive = switchElement.getAttribute('data-active') === 'true';

        // Toggle state
        switchElement.setAttribute('data-active', !isActive);

        // Update switch text and visual state
        const textElement = element.querySelector('span');
        if (!isActive) {
            textElement.textContent = 'Show populated only';
            switchElement.style.backgroundColor = '#1e3a8a';
            switchElement.style.color = 'white';
        } else {
            textElement.textContent = 'Show all fields';
            switchElement.style.backgroundColor = '#e5e7eb';
            switchElement.style.color = '#6b7280';
        }

        // Apply filter to table rows - look for the modern table structure
        const tableContainer = element.closest('.fields-compact').querySelector('.modern-hl7-table');
        if (!tableContainer) {
            console.warn(`No table container found for segment ${segName}`);
            return;
        }

        const rows = tableContainer.querySelectorAll('tbody tr');
        let visibleCount = 0;
        let hiddenCount = 0;

        rows.forEach(row => {
            const fieldId = row.getAttribute('data-field-id');
            const hasValue = row.querySelector('td:first-child .field-with-value') ||
                           row.textContent.includes('●') || // Look for green data indicator
                           !row.textContent.includes('No value'); // Check if not "No value"

            if (!isActive) {
                // Show only populated fields
                if (hasValue) {
                    row.style.display = '';
                    visibleCount++;
                } else {
                    row.style.display = 'none';
                    hiddenCount++;
                }
            } else {
                // Show all fields
                row.style.display = '';
                visibleCount++;
            }
        });

        // Update the fields count display - look for the modern structure
        const segmentContainer = element.closest('.segment-viewer, .fields-section, .segment-container, .parsing-results');
        const fieldsInfo = segmentContainer?.querySelector('.fields-count, .field-count-display, .fields-header .field-stats');

        if (fieldsInfo) {
            const totalFields = rows.length;
            if (!isActive) {
                fieldsInfo.textContent = `Fields (${visibleCount} populated of ${totalFields})`;
                fieldsInfo.style.color = '#059669';
            } else {
                fieldsInfo.textContent = `Fields (${totalFields})`;
                fieldsInfo.style.color = '#4b5563';
            }
        } else {
            console.log('🔍 Fields info element not found. Available containers:', segmentContainer?.className || 'none');
        }

        console.log(`🔄 Field visibility toggled for ${segName}: ${!isActive ? 'populated only' : 'all fields'} - Showing ${visibleCount}, Hidden ${hiddenCount}`);
    }

    /**
     * Sort fields by their HL7 position for correct display order
     */
    sortFieldsByPosition(fields) {
        if (!Array.isArray(fields)) {
            console.warn('Fields is not an array:', fields);
            return [];
        }

        // Create a copy to avoid modifying the original
        const sortedFields = [...fields];

        // Sort by position (primary) and sequence (secondary)
        sortedFields.sort((a, b) => {
            // Primary sort: by position
            if (a.position !== b.position) {
                return a.position - b.position;
            }
            // Secondary sort: by sequence if positions are equal
            return (a.sequence || 0) - (b.sequence || 0);
        });

        console.log(`🔀 Sorted ${sortedFields.length} fields by position for display`);
        return sortedFields;
    }

    /**
     * Render field value with component parsing (^, ~, &, \ separators)
     */
    renderFieldValueWithComponents(field) {
        const hasValue = field.value && field.value.trim() !== '';

        if (!hasValue) {
            return '<div style="font-size: 12px; color: #9ca3af; font-style: italic;">No value</div>';
        }

        const value = field.value;
        let html = '';

        // Check if value has components (^ separator)
        if (value.includes('^')) {
            const components = value.split('^');
            html += '<div style="font-size: 12px; color: #4b5563;">';
            html += '<div style="margin-bottom: 4px;"><strong>Components:</strong></div>';

            components.forEach((component, idx) => {
                if (component.trim()) {
                    html += `<div style="margin-left: 12px; margin-bottom: 2px;">
                        <span style="font-family: monospace; color: #6b7280; font-size: 10px;">${idx + 1}:</span>
                        <span style="margin-left: 4px;">${this.escapeHtml(component)}</span>
                    </div>`;
                }
            });
            html += '</div>';
        }
        // Check if value has repetitions (~ separator)
        else if (value.includes('~')) {
            const repetitions = value.split('~');
            html += '<div style="font-size: 12px; color: #4b5563;">';
            html += '<div style="margin-bottom: 4px;"><strong>Repetitions:</strong></div>';

            repetitions.forEach((rep, idx) => {
                if (rep.trim()) {
                    html += `<div style="margin-left: 12px; margin-bottom: 2px;">
                        <span style="font-family: monospace; color: #6b7280; font-size: 10px;">${idx + 1}:</span>
                        <span style="margin-left: 4px;">${this.escapeHtml(rep)}</span>
                    </div>`;
                }
            });
            html += '</div>';
        }
        // Check if value has subcomponents (& separator)
        else if (value.includes('&')) {
            const subcomponents = value.split('&');
            html += '<div style="font-size: 12px; color: #4b5563;">';
            html += '<div style="margin-bottom: 4px;"><strong>Subcomponents:</strong></div>';

            subcomponents.forEach((sub, idx) => {
                if (sub.trim()) {
                    html += `<div style="margin-left: 12px; margin-bottom: 2px;">
                        <span style="font-family: monospace; color: #6b7280; font-size: 10px;">${idx + 1}:</span>
                        <span style="margin-left: 4px;">${this.escapeHtml(sub)}</span>
                    </div>`;
                }
            });
            html += '</div>';
        }
        // Simple value
        else {
            html += `<div style="font-size: 12px; color: #4b5563; font-family: monospace; background: #f9fafb; padding: 6px 8px; border-radius: 4px; border-left: 3px solid #3b82f6;">
                ${this.escapeHtml(value)}
            </div>`;
        }

        return html;
    }

    /**
     * Toggle between showing all fields vs only fields with values
     */
    toggleAllFields(element) {
        const switchElement = element.querySelector('.field-filter-switch');
        const isActive = switchElement.getAttribute('data-active') === 'true';

        // Toggle state
        switchElement.setAttribute('data-active', (!isActive).toString());
        if (!isActive) {
            switchElement.classList.add('active');
            element.querySelector('span').textContent = 'Show populated only';
        } else {
            switchElement.classList.remove('active');
            element.querySelector('span').textContent = 'Show all fields';
        }

        // Toggle field visibility
        const container = element.closest('.segment-content') || element.closest('#segmentsContainer');
        if (!isActive) {
            // Show all fields
            container.classList.add('show-all-fields');
        } else {
            // Show only populated fields
            container.classList.remove('show-all-fields');
        }
    }

    /**
     * Escape HTML to prevent XSS
     */
    escapeHtml(text) {
        if (!text) return '';
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    /**
     * Toggle segment expansion (from backup design)
     */
    toggleSegment(segName) {
        if (!this.expandedSegments) {
            this.expandedSegments = new Set(['MSH', 'PID']);
        }

        if (this.expandedSegments.has(segName)) {
            this.expandedSegments.delete(segName);
        } else {
            this.expandedSegments.add(segName);
        }

        this.refreshView();
    }

    /**
     * Toggle all segments
     */
    toggleAllSegments() {
        if (!this.expandedSegments) {
            this.expandedSegments = new Set();
        }

        // Get available segments
        const availableSegments = ['MSH', 'PID', 'PV1']; // Default important segments
        const hasExpanded = availableSegments.some(seg => this.expandedSegments.has(seg));

        if (hasExpanded) {
            availableSegments.forEach(seg => this.expandedSegments.delete(seg));
        } else {
            availableSegments.forEach(seg => this.expandedSegments.add(seg));
        }

        this.refreshView();
    }

    /**
     * Set view mode
     */
    setViewMode(mode) {
        this.viewMode = mode;
        this.refreshView();
    }

    /**
     * Refresh the current view
     */
    refreshView() {
        // This would refresh the display - for now, just log
        console.log('View refresh requested');
    }

    /**
     * Render dynamic badges (from backup design)
     */
    renderDynamicBadges(segName, segment, segmentErrors) {
        const badges = [];

        if (segmentErrors.some(err => err.severity === 'ERROR' || err.severity === 'error')) {
            badges.push('<span class="badge-mini error" title="Has errors">!</span>');
        } else if (segmentErrors.some(err => err.severity === 'WARNING' || err.severity === 'warning')) {
            badges.push('<span class="badge-mini warning" title="Has warnings">⚠</span>');
        }

        if (segment.required === true) {
            badges.push('<span class="badge-mini required" title="Required segment">R</span>');
        }

        if (segName.startsWith('Z')) {
            badges.push('<span class="badge-mini custom" title="Custom segment">Z</span>');
        }

        return badges.join('');
    }

    /**
     * Render inline validation (from backup design)
     */
    renderInlineValidation(segmentErrors) {
        if (segmentErrors.length === 0) return '';

        const firstError = segmentErrors[0];
        return `
            <div class="inline-validation">
                <span class="validation-icon">${firstError.severity === 'error' || firstError.severity === 'ERROR' ? '❌' : '⚠️'}</span>
                <span class="validation-text">${this.truncateText(firstError.message, 60)}</span>
                ${segmentErrors.length > 1 ? `<span class="more-count">+${segmentErrors.length - 1} more</span>` : ''}
            </div>
        `;
    }

    /**
     * Get key fields (from backup design)
     */
    getKeyFields(segName, segment) {
        if (!segment.fields || segment.fields.length === 0) {
            return [];
        }

        const keyFields = [];
        const fieldsWithValues = segment.fields
            .filter(field => field.hasValue && field.value)
            .slice(0, 3);

        fieldsWithValues.forEach(field => {
            keyFields.push(`${field.key}:${this.truncateValue(field.value, 15)}`);
        });

        if (keyFields.length === 0) {
            segment.fields.slice(0, 2).forEach(field => {
                keyFields.push(`${field.key}:—`);
            });
        }

        return keyFields;
    }

    /**
     * Truncate text helper
     */
    truncateText(text, maxLength) {
        if (!text) return '';
        return text.length > maxLength ? text.substring(0, maxLength) + '...' : text;
    }

    /**
     * Truncate value helper
     */
    truncateValue(value, maxLength = 30) {
        if (!value) return '';
        const str = String(value);
        return str.length > maxLength ? str.substring(0, maxLength) + '...' : str;
    }

    /**
     * Extract atomic mappings from parsed HL7 data
     */
    extractAtomicMappingsFromParsedData(parsedHL7Data) {
        if (!parsedHL7Data?.enhancedSegments) return [];

        const mappings = [];
        const knownMappings = this.getKnownHL7ToFHIRMappings();

        Object.entries(parsedHL7Data.enhancedSegments).forEach(([segmentName, segmentData]) => {
            if (segmentData.fields && Array.isArray(segmentData.fields)) {
                segmentData.fields.forEach(field => {
                    if (field.value && field.key) {
                        const resourceType = this.getResourceTypeForHL7Field(field.key);
                        const fhirPath = this.getFHIRPathForHL7Field(field.key, knownMappings);

                        if (resourceType && fhirPath) {
                            mappings.push({
                                hl7Field: field.key,
                                fhirPath: fhirPath,
                                value: field.value,
                                resourceType: resourceType,
                                fieldName: field.name || field.key,
                                description: field.description || ''
                            });
                        }
                    }

                    // Process subfields
                    if (field.subfields && Array.isArray(field.subfields)) {
                        field.subfields.forEach(subfield => {
                            if (subfield.value && subfield.key) {
                                const resourceType = this.getResourceTypeForHL7Field(subfield.key);
                                const fhirPath = this.getFHIRPathForHL7Field(subfield.key, knownMappings);

                                if (resourceType && fhirPath) {
                                    mappings.push({
                                        hl7Field: subfield.key,
                                        fhirPath: fhirPath,
                                        value: subfield.value,
                                        resourceType: resourceType,
                                        fieldName: subfield.name || subfield.key,
                                        description: subfield.description || ''
                                    });
                                }
                            }
                        });
                    }
                });
            }
        });

        return mappings;
    }

    /**
     * Get transformation summary section
     */
    getTransformationSummarySection(messageType, segments, fields, mappingsCount) {
        const resourceCount = this.getResourceTypesFromMappings([]).length || 3; // Default estimate
        const validationScore = mappingsCount > 0 ? Math.round((mappingsCount / (mappingsCount + 0)) * 100) + '%' : '100%';

        return `
            <div style="background: linear-gradient(135deg, #f8f9ff 0%, #fff 100%); border: 2px solid #e2e8f0; border-radius: 12px; padding: 20px; margin-bottom: 24px;">
                <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px;">
                    <h4 style="margin: 0; font-size: 16px; font-weight: 600; color: #1e3a8a;">HL7 → FHIR Transformation</h4>
                    <span style="padding: 4px 12px; background: #10b981; color: white; border-radius: 20px; font-size: 12px; font-weight: 600;">
                        ✓ Success
                    </span>
                </div>

                <div style="display: grid; grid-template-columns: repeat(4, 1fr); gap: 16px;">
                    <div style="text-align: center;">
                        <div style="font-size: 24px; font-weight: 700; color: #1e3a8a;">${messageType}</div>
                        <div style="font-size: 12px; color: #6b7280; margin-top: 4px;">Message Type</div>
                    </div>
                    <div style="text-align: center;">
                        <div style="font-size: 24px; font-weight: 700; color: #10b981;">${resourceCount}</div>
                        <div style="font-size: 12px; color: #6b7280; margin-top: 4px;">Resources</div>
                    </div>
                    <div style="text-align: center;">
                        <div style="font-size: 24px; font-weight: 700; color: #3b82f6;">${mappingsCount}</div>
                        <div style="font-size: 12px; color: #6b7280; margin-top: 4px;">Mappings</div>
                    </div>
                    <div style="text-align: center;">
                        <div style="font-size: 24px; font-weight: 700; color: #10b981;">${validationScore}</div>
                        <div style="font-size: 12px; color: #6b7280; margin-top: 4px;">Validation</div>
                    </div>
                </div>

                <div style="display: flex; gap: 8px; margin-top: 16px; padding-top: 16px; border-top: 1px solid #e2e8f0;">
                    <button onclick="window.wizardView.expandAllResources()" style="padding: 6px 12px; background: white; border: 1px solid #e2e8f0; border-radius: 6px; font-size: 12px; cursor: pointer;">
                        📂 Expand All
                    </button>
                    <button onclick="window.wizardView.collapseAllResources()" style="padding: 6px 12px; background: white; border: 1px solid #e2e8f0; border-radius: 6px; font-size: 12px; cursor: pointer;">
                        📁 Collapse All
                    </button>
                    <button onclick="window.wizardView.viewRawJson()" style="padding: 6px 12px; background: white; border: 1px solid #e2e8f0; border-radius: 6px; font-size: 12px; cursor: pointer;">
                        👁️ View JSON
                    </button>
                    <button onclick="window.wizardView.proceedToTransform()" style="padding: 6px 12px; background: #3b82f6; color: white; border: none; border-radius: 6px; font-size: 12px; cursor: pointer;">
                        ⚡ Transform to FHIR
                    </button>
                </div>
            </div>
        `;
    }

    /**
     * Get HL7 element table section with beautiful resource cards
     */
    getHL7ElementTableSection(atomicMappings, resourceTypes) {
        if (atomicMappings.length === 0) {
            return '<div style="text-align: center; padding: 40px; color: #6b7280;">No field mappings to display</div>';
        }

        let html = '<div id="resourcesContainer" style="margin-top: 16px;">';

        resourceTypes.forEach((resourceType, index) => {
            const mappings = atomicMappings.filter(m => m.resourceType === resourceType);
            html += this.generateResourceCard(resourceType, mappings, index);
        });

        html += '</div>';
        return html;
    }

    /**
     * Generate resource card with mappings table (adapted from backup)
     */
    generateResourceCard(resourceType, mappings, index) {
        const icon = this.getResourceIcon(resourceType);
        const mappingCount = mappings.length;

        return `
            <div style="background: white; border: 1px solid #e2e8f0; border-radius: 12px; overflow: hidden; margin-bottom: 16px;">
                <div onclick="window.wizardView.toggleResource(${index})" style="padding: 16px; background: linear-gradient(135deg, #f8f9ff 0%, #fff 100%); cursor: pointer; display: flex; justify-content: space-between; align-items: center;">
                    <div style="display: flex; align-items: center; gap: 12px;">
                        <span style="font-size: 24px;">${icon}</span>
                        <div>
                            <h5 style="margin: 0; font-size: 16px; font-weight: 600; color: #1e3a8a;">${resourceType}</h5>
                            <div style="font-size: 12px; color: #6b7280;">${mappingCount} field mappings</div>
                        </div>
                    </div>
                    <span id="toggle-icon-${index}" style="font-size: 20px; color: #6b7280;">▼</span>
                </div>

                <div id="resource-content-${index}" style="padding: 16px; display: block;">
                    ${this.generateMappingsTable(mappings)}
                </div>
            </div>
        `;
    }

    /**
     * Generate mappings table (adapted from backup)
     */
    generateMappingsTable(mappings) {
        if (mappings.length === 0) {
            return '<div style="text-align: center; color: #6b7280; padding: 20px;">No mappings found</div>';
        }

        let html = '<div style="overflow-x: auto;">';
        html += '<table style="width: 100%; border-collapse: collapse;">';
        html += '<thead><tr style="border-bottom: 2px solid #e2e8f0;">';
        html += '<th style="text-align: left; padding: 12px; font-size: 12px; font-weight: 600; color: #6b7280;">HL7 Field</th>';
        html += '<th style="text-align: center; padding: 12px; font-size: 12px; font-weight: 600; color: #6b7280;">→</th>';
        html += '<th style="text-align: left; padding: 12px; font-size: 12px; font-weight: 600; color: #6b7280;">FHIR Path</th>';
        html += '<th style="text-align: left; padding: 12px; font-size: 12px; font-weight: 600; color: #6b7280;">Value</th>';
        html += '<th style="text-align: center; padding: 12px; font-size: 12px; font-weight: 600; color: #6b7280;">Type</th>';
        html += '</tr></thead>';
        html += '<tbody>';

        mappings.forEach((mapping, idx) => {
            const hl7Parts = mapping.hl7Field.split('.');
            const hl7Display = hl7Parts[0] + '.' + hl7Parts.slice(1).join('.');
            const statusColor = '#10b981';
            const statusLabel = 'Direct';

            html += `
                <tr style="border-bottom: 1px solid #f3f4f6;">
                    <td style="padding: 12px;">
                        <div style="font-family: monospace; font-size: 12px; color: #4b5563; font-weight: 600;">${hl7Display}</div>
                        ${mapping.fieldName ? `<div style="font-size: 10px; color: #9ca3af; margin-top: 2px;">${mapping.fieldName}</div>` : ''}
                    </td>
                    <td style="text-align: center; padding: 12px;">
                        <span style="color: #3b82f6; font-weight: bold;">→</span>
                    </td>
                    <td style="padding: 12px;">
                        <div style="font-family: monospace; font-size: 12px; color: #1e3a8a; font-weight: 600;">${mapping.fhirPath}</div>
                    </td>
                    <td style="padding: 12px;">
                        <div style="font-size: 12px; color: #4b5563; max-width: 200px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;" title="${mapping.value}">${mapping.value}</div>
                    </td>
                    <td style="text-align: center; padding: 12px;">
                        <span style="padding: 2px 8px; background: ${statusColor}; color: white; border-radius: 4px; font-size: 10px; font-weight: 600;">
                            ${statusLabel}
                        </span>
                    </td>
                </tr>
            `;
        });

        html += '</tbody></table></div>';
        return html;
    }

    /**
     * Generate segments display
     */
    getSegmentsDisplay(parsedData) {
        if (!parsedData?.segments) {
            return '<div class="no-segments">No segments found</div>';
        }

        return Object.entries(parsedData.segments).map(([segmentName, segmentData]) => `
            <div class="segment-card">
                <div class="segment-header" onclick="this.toggleSegment('${segmentName}')">
                    <div class="segment-title">
                        <div class="segment-icon">${this.getSegmentIcon(segmentName)}</div>
                        <div class="segment-info">
                            <h5>${segmentName}</h5>
                            <p>${this.getSegmentDescription(segmentName)}</p>
                        </div>
                    </div>
                    <div class="segment-toggle">
                        <span class="expand-icon">▼</span>
                    </div>
                </div>
                <div class="segment-content" style="display: none;">
                    ${this.getSegmentFields(segmentName, segmentData, parsedData.enhancedSegments?.[segmentName])}
                </div>
            </div>
        `).join('');
    }

    /**
     * Get segment icon
     */
    getSegmentIcon(segmentName) {
        const icons = {
            'MSH': '📨',
            'PID': '👤',
            'PV1': '🏥',
            'EVN': '⚡',
            'NK1': '👥',
            'OBX': '🔬',
            'ORC': '📋',
            'AL1': '⚠️',
            'DG1': '🩺'
        };
        return icons[segmentName] || '📄';
    }

    /**
     * Get segment description
     */
    getSegmentDescription(segmentName) {
        const descriptions = {
            'MSH': 'Message Header',
            'PID': 'Patient Identification',
            'PV1': 'Patient Visit',
            'EVN': 'Event Type',
            'NK1': 'Next of Kin',
            'OBX': 'Observation Result',
            'ORC': 'Common Order',
            'AL1': 'Patient Allergy',
            'DG1': 'Diagnosis'
        };
        return descriptions[segmentName] || 'Unknown Segment';
    }

    /**
     * Get segment fields display
     */
    getSegmentFields(segmentName, segmentData, enhancedData) {
        if (!segmentData || typeof segmentData !== 'object') {
            return '<div class="no-fields">No field data available</div>';
        }

        // Use enhanced data if available, otherwise use basic segment data
        const fields = enhancedData?.fields || [];

        if (fields.length === 0) {
            // Fallback to displaying raw segment data
            return `<div class="raw-segment-data"><pre>${JSON.stringify(segmentData, null, 2)}</pre></div>`;
        }

        return `
            <div class="fields-grid">
                ${fields.map((field, index) => `
                    <div class="field-item">
                        <div class="field-header">
                            <span class="field-position">${segmentName}.${field.position || index + 1}</span>
                            <span class="field-name">${field.name || 'Field ' + (index + 1)}</span>
                        </div>
                        <div class="field-value">${field.value || 'N/A'}</div>
                        ${field.description ? `<div class="field-description">${field.description}</div>` : ''}
                    </div>
                `).join('')}
            </div>
        `;
    }

    /**
     * Generate OOB mapping templates
     */
    getOOBMappingTemplates(templates = {}, selectedTemplate = '') {
        return Object.entries(templates).map(([id, template]) => `
            <div class="oob-template-item ${selectedTemplate === id ? 'selected' : ''}"
                 data-template-id="${id}">
                <div class="oob-template-header">
                    <input type="radio" name="mappingTemplate" value="${id}"
                           id="template_${id}" ${selectedTemplate === id ? 'checked' : ''}>
                    <label for="template_${id}" class="oob-template-title">${template.name}</label>
                    <span class="oob-badge">OOB</span>
                </div>
                <div class="oob-template-description">${template.description}</div>
                <div class="oob-template-stats">
                    <span class="stat">📋 ${template.mappings?.length || 0} mappings</span>
                    <span class="stat">⚡ Ready to use</span>
                </div>
            </div>
        `).join('');
    }

    /**
     * Visual mapping panel
     */
    getVisualMappingPanel(mappings = []) {
        return `
            <div class="visual-mapping-container">
                <div class="mapping-columns">
                    <div class="mapping-column hl7-column">
                        <h4>📥 HL7 Source</h4>
                        <div class="field-list" id="hl7Fields">
                            <!-- Dynamically populated -->
                        </div>
                    </div>

                    <div class="mapping-column connections">
                        <div class="mapping-connections" id="mappingConnections">
                            <!-- SVG connections -->
                        </div>
                        <button class="btn btn-outline-primary add-mapping-btn" id="addMapping">
                            <span>+</span> Add Mapping
                        </button>
                    </div>

                    <div class="mapping-column fhir-column">
                        <h4>📤 FHIR Target</h4>
                        <div class="field-list" id="fhirFields">
                            <!-- Dynamically populated -->
                        </div>
                    </div>
                </div>

                <div class="mapping-controls">
                    <button class="btn btn-secondary" id="validateMappings">
                        <span class="btn-icon">✓</span>
                        Validate Mappings
                    </button>
                    <button class="btn btn-outline-primary" id="testTransform">
                        <span class="btn-icon">🧪</span>
                        Test Transform
                    </button>
                </div>
            </div>
        `;
    }

    /**
     * Table mapping panel
     */
    getTableMappingPanel(mappings = []) {
        return `
            <div class="table-mapping-container">
                <div class="mapping-table-controls">
                    <button class="btn btn-primary" id="addMappingRow">
                        <span>+</span> Add Mapping
                    </button>
                    <button class="btn btn-outline-secondary" id="importMappings">
                        📁 Import
                    </button>
                    <button class="btn btn-outline-secondary" id="exportMappings">
                        💾 Export
                    </button>
                </div>

                <div class="mapping-table-wrapper">
                    <table class="mapping-table" id="mappingTable">
                        <thead>
                            <tr>
                                <th>HL7 Path</th>
                                <th>FHIR Path</th>
                                <th>Required</th>
                                <th>Transform</th>
                                <th>Actions</th>
                            </tr>
                        </thead>
                        <tbody id="mappingTableBody">
                            ${this.getMappingTableRows(mappings)}
                        </tbody>
                    </table>
                </div>
            </div>
        `;
    }

    /**
     * Generate mapping table rows
     */
    getMappingTableRows(mappings = []) {
        if (mappings.length === 0) {
            return `
                <tr class="empty-row">
                    <td colspan="5" class="text-center">
                        <div class="empty-state">
                            <span class="empty-icon">📋</span>
                            <p>No mappings defined yet</p>
                            <p class="text-muted">Add mappings to configure message transformation</p>
                        </div>
                    </td>
                </tr>
            `;
        }

        return mappings.map((mapping, index) => `
            <tr data-mapping-index="${index}">
                <td>
                    <input type="text" class="form-control" value="${mapping.hl7Path || ''}"
                           placeholder="e.g., PID.5.1" data-field="hl7Path">
                </td>
                <td>
                    <input type="text" class="form-control" value="${mapping.fhirPath || ''}"
                           placeholder="e.g., Patient.name[0].family" data-field="fhirPath">
                </td>
                <td>
                    <div class="form-check">
                        <input type="checkbox" class="form-check-input"
                               ${mapping.required ? 'checked' : ''} data-field="required">
                    </div>
                </td>
                <td>
                    <select class="form-control" data-field="transform">
                        <option value="direct" ${mapping.transform === 'direct' ? 'selected' : ''}>Direct</option>
                        <option value="uppercase" ${mapping.transform === 'uppercase' ? 'selected' : ''}>Uppercase</option>
                        <option value="lowercase" ${mapping.transform === 'lowercase' ? 'selected' : ''}>Lowercase</option>
                        <option value="date" ${mapping.transform === 'date' ? 'selected' : ''}>Date Format</option>
                        <option value="custom" ${mapping.transform === 'custom' ? 'selected' : ''}>Custom</option>
                    </select>
                </td>
                <td>
                    <button type="button" class="btn btn-sm btn-outline-danger"
                            onclick="this.closest('tr').remove()" title="Delete">
                        <span>🗑️</span>
                    </button>
                </td>
            </tr>
        `).join('');
    }

    /**
     * Preview mapping panel
     */
    getPreviewMappingPanel() {
        return `
            <div class="preview-container">
                <div class="preview-controls">
                    <button class="btn btn-primary" id="generatePreview">
                        <span class="btn-icon">🔄</span>
                        Generate Preview
                    </button>
                    <div class="preview-options">
                        <label class="form-check">
                            <input type="checkbox" id="includeEmptyFields" checked>
                            Include empty fields
                        </label>
                        <label class="form-check">
                            <input type="checkbox" id="prettyPrint" checked>
                            Pretty print JSON
                        </label>
                    </div>
                </div>

                <div class="preview-panels">
                    <div class="preview-panel">
                        <h4>Sample HL7 Input</h4>
                        <textarea id="sampleHL7" class="preview-textarea" readonly>
MSH|^~\\&amp;|EPIC|HOSPITAL|FHIR|DESTINATION|20230101120000||ADT^A01|123456|P|2.5
PID|1||12345^^^MRN||Doe^John^M||19800101|M|||123 Main St^^Anytown^ST^12345
PV1|1|I|ICU^101^1|||DOC123^Smith^Jane|||EMERGENCY|||
                        </textarea>
                    </div>

                    <div class="preview-panel">
                        <h4>Generated FHIR Output</h4>
                        <div id="fhirPreview" class="preview-json"></div>
                    </div>
                </div>
            </div>
        `;
    }

    /**
     * Step 6: Summary and Completion
     */
    getStep6Template(data) {
        return `
            <div class="wizard-step-content" data-step="6">
                <div class="wizard-step-header">
                    <h3>Summary & Completion</h3>
                    <p>Review your interface configuration before creation</p>
                </div>

                <div class="summary-content">
                    <div class="summary-section">
                        <h4>📝 Basic Information</h4>
                        <div class="summary-item">
                            <label>Name:</label>
                            <span>${data.name || 'N/A'}</span>
                        </div>
                        <div class="summary-item">
                            <label>Description:</label>
                            <span>${data.description || 'Auto-generated'}</span>
                        </div>
                    </div>

                    <div class="summary-section">
                        <h4>📥 Source Configuration</h4>
                        <div class="summary-item">
                            <label>Type:</label>
                            <span class="badge badge-primary">${data.sourceType}</span>
                        </div>
                        <div class="summary-item">
                            <label>Connectivity:</label>
                            <span class="badge badge-secondary">${data.sourceConnectivity}</span>
                        </div>
                        ${this.getSummaryConfig('source', data.sourceConfig)}
                    </div>

                    <div class="summary-section">
                        <h4>📤 Target Configuration</h4>
                        <div class="summary-item">
                            <label>Type:</label>
                            <span class="badge badge-success">${data.targetType}</span>
                        </div>
                        <div class="summary-item">
                            <label>Connectivity:</label>
                            <span class="badge badge-secondary">${data.targetConnectivity}</span>
                        </div>
                        ${this.getSummaryConfig('target', data.targetConfig)}
                    </div>

                    <div class="summary-section">
                        <h4>🔄 Mapping Configuration</h4>
                        <div class="summary-item">
                            <label>Message Type:</label>
                            <span class="badge badge-info">${data.messageType}</span>
                        </div>
                        <div class="summary-item">
                            <label>Template Used:</label>
                            <span>${data.mappingTemplate ? data.mappingTemplate + ' (OOB)' : 'Custom'}</span>
                        </div>
                        <div class="summary-item">
                            <label>Total Mappings:</label>
                            <span class="badge badge-warning">${data.customMappings?.length || 0}</span>
                        </div>
                    </div>

                    <!-- Configuration Actions -->
                    <div class="summary-actions">
                        <div class="action-group">
                            <button class="btn btn-outline-secondary" id="exportConfig">
                                <span class="btn-icon">💾</span>
                                Export Configuration
                            </button>
                            <button class="btn btn-outline-primary" id="testFullTransform">
                                <span class="btn-icon">🧪</span>
                                Final Test
                            </button>
                        </div>

                        <div class="creation-options">
                            <label class="form-check">
                                <input type="checkbox" id="activateAfterCreation" checked>
                                Activate interface after creation
                            </label>
                            <label class="form-check">
                                <input type="checkbox" id="notifyOnErrors">
                                Send email notifications on errors
                            </label>
                        </div>
                    </div>
                </div>
            </div>
        `;
    }

    /**
     * Generate summary configuration details
     */
    getSummaryConfig(type, config = {}) {
        const configItems = Object.entries(config)
            .filter(([key, value]) => value && key !== 'password' && key !== 'token')
            .map(([key, value]) => `
                <div class="summary-item">
                    <label>${this.formatConfigKey(key)}:</label>
                    <span>${value}</span>
                </div>
            `).join('');

        return configItems || '<div class="summary-item text-muted">Using default configuration</div>';
    }

    /**
     * Format configuration key for display
     */
    formatConfigKey(key) {
        return key.replace(/([A-Z])/g, ' $1')
                  .replace(/^./, str => str.toUpperCase())
                  .replace(/([a-z])([A-Z])/g, '$1 $2');
    }

    /**
     * Update step indicator
     */
    updateStepIndicator(currentStep) {
        console.log('🎯 UpdateStepIndicator called with currentStep:', currentStep);
        // Try both selectors - creative and standard
        let stepElements = document.querySelectorAll('.wizard-step-creative');
        if (stepElements.length === 0) {
            stepElements = document.querySelectorAll('.wizard-step');
        }
        console.log('🎯 Found step elements:', stepElements.length);

        stepElements.forEach((element, index) => {
            const stepNumber = index + 1;
            element.classList.remove('active', 'completed');

            if (stepNumber === currentStep) {
                console.log(`🎯 Setting step ${stepNumber} as ACTIVE`);
                element.classList.add('active');
                element.setAttribute('aria-selected', 'true');
                element.setAttribute('tabindex', '0');
            } else if (stepNumber < currentStep) {
                element.classList.add('completed');
                element.setAttribute('aria-selected', 'false');
                element.setAttribute('tabindex', '-1');
            } else {
                element.setAttribute('aria-selected', 'false');
                element.setAttribute('tabindex', '-1');
            }
        });

        // Update progress bar
        const progressFill = document.getElementById('wizardProgressFill');
        const creativeProgressFill = document.querySelector('.progress-fill');

        if (progressFill) {
            const progress = ((currentStep - 1) / 4) * 100;
            progressFill.style.width = `${progress}%`;
        } else if (creativeProgressFill) {
            // Update creative SVG progress
            const progress = ((currentStep - 1) / 3) * 100; // 4 steps = 3 intervals
            const dashOffset = 1000 - (1000 * progress / 100);
            console.log('🎯 Setting creative progress SVG to:', progress + '% (dashOffset:', dashOffset + ')');
            creativeProgressFill.style.strokeDashoffset = dashOffset;
        } else {
            console.log('🎯 No progress elements found (neither standard nor creative)');
        }
    }

    /**
     * Update navigation buttons
     */
    updateNavigation(currentStep, validation = {}) {
        const prevBtn = document.getElementById('wizardPrevious');
        const nextBtn = document.getElementById('wizardNext');
        const finishBtn = document.getElementById('wizardFinish');

        if (prevBtn) {
            prevBtn.disabled = currentStep === 1;
        }

        if (nextBtn && finishBtn) {
            if (currentStep === 4) {
                nextBtn.style.display = 'none';
                finishBtn.style.display = 'inline-flex';
                finishBtn.disabled = !validation.isValid;
            } else {
                nextBtn.style.display = 'inline-flex';
                finishBtn.style.display = 'none';
                nextBtn.disabled = !validation.isValid;
            }
        }
    }

    /**
     * Update help content
     */
    updateHelp(stepNumber) {
        const helpContent = document.getElementById('wizardHelp');
        if (!helpContent) return;

        const helpTexts = {
            1: {
                title: 'Getting Started',
                content: `
                    <p>Choose a descriptive name for your interface and optionally provide a description.</p>
                    <p><strong>💡 Quick Start:</strong> Use our OOB templates for common scenarios:</p>
                    <ul>
                        <li><strong>Hospital ADT:</strong> Patient admission/discharge messages</li>
                        <li><strong>Lab Results:</strong> Laboratory reporting</li>
                        <li><strong>Radiology:</strong> Imaging and diagnostic reports</li>
                    </ul>
                `
            },
            2: {
                title: 'Source Configuration',
                content: `
                    <p>Configure where your HL7 messages come from.</p>
                    <p><strong>💡 OOB Defaults:</strong></p>
                    <ul>
                        <li><strong>HL7 v2.x:</strong> Most common format in healthcare</li>
                        <li><strong>TCP/MLLP:</strong> Standard HL7 transport protocol</li>
                        <li><strong>Port 2575:</strong> Standard HL7 listening port</li>
                    </ul>
                    <p>Test your connection to ensure messages can be received.</p>
                `
            },
            3: {
                title: 'Target Configuration',
                content: `
                    <p>Configure your FHIR server where transformed data will be sent.</p>
                    <p><strong>💡 OOB Defaults:</strong></p>
                    <ul>
                        <li><strong>FHIR R4:</strong> Latest stable FHIR version</li>
                        <li><strong>HTTP/REST:</strong> Standard FHIR API protocol</li>
                        <li><strong>JSON Format:</strong> Most common FHIR format</li>
                    </ul>
                    <p>Test your FHIR server connection for validation.</p>
                `
            },
            4: {
                title: 'Mapping Configuration',
                content: `
                    <p>Define how HL7 segments map to FHIR resources.</p>
                    <p><strong>💡 OOB Templates:</strong> Start with pre-built mappings for common message types.</p>
                    <p><strong>Visual Mode:</strong> Drag and drop interface for easy mapping creation.</p>
                    <p><strong>Table Mode:</strong> Detailed mapping configuration with transformations.</p>
                    <p>Always test your mappings before completion.</p>
                `
            },
            5: {
                title: 'Ready to Deploy',
                content: `
                    <p>Review your configuration and test the complete transformation.</p>
                    <p><strong>💡 Recommendations:</strong></p>
                    <ul>
                        <li>Export your configuration for backup</li>
                        <li>Run a final transformation test</li>
                        <li>Enable activation for immediate use</li>
                        <li>Set up error notifications</li>
                    </ul>
                    <p>Your interface will be ready to process messages immediately after creation.</p>
                `
            }
        };

        const help = helpTexts[stepNumber];
        if (help) {
            helpContent.innerHTML = `
                <div class="help-content">
                    <h4>${help.title}</h4>
                    ${help.content}
                </div>
            `;
        }
    }

    /**
     * Setup accessibility features
     */
    setupAccessibility() {
        // Focus management
        this.modal.setAttribute('tabindex', '-1');

        // Escape key handler
        document.addEventListener('keydown', (e) => {
            if (e.key === 'Escape' && this.isVisible()) {
                this.dispatchEvent(new CustomEvent('close'));
            }
        });

        // Focus trap
        this.setupFocusTrap();
    }

    /**
     * Setup focus trap for modal
     */
    setupFocusTrap() {
        const focusableElements = this.modal.querySelectorAll(
            'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
        );

        if (focusableElements.length === 0) return;

        const firstElement = focusableElements[0];
        const lastElement = focusableElements[focusableElements.length - 1];

        this.modal.addEventListener('keydown', (e) => {
            if (e.key === 'Tab') {
                if (e.shiftKey) {
                    if (document.activeElement === firstElement) {
                        e.preventDefault();
                        lastElement.focus();
                    }
                } else {
                    if (document.activeElement === lastElement) {
                        e.preventDefault();
                        firstElement.focus();
                    }
                }
            }
        });
    }

    /**
     * Setup step-specific event listeners
     */
    setupStepEventListeners(stepContainer) {
        const step = stepContainer.getAttribute('data-step');
        console.log('🔧 setupStepEventListeners called with:', {
            containerTagName: stepContainer.tagName,
            containerId: stepContainer.id,
            containerClass: stepContainer.className,
            dataStep: step,
            hasDataStep: stepContainer.hasAttribute('data-step')
        });

        switch (step) {
            case '1':
                this.setupStep1Listeners(stepContainer);
                break;
            case '2':
                console.log('🎯 About to setup Step 2 listeners (HL7 Parsing)');
                this.setupStep2Listeners(stepContainer);
                break;
            case '3':
                this.setupStep3Listeners(stepContainer);
                break;
            case '4':
                this.setupStep4Listeners(stepContainer);
                break;
            case '5':
                this.setupStep5Listeners(stepContainer);
                break;
            case '6':
                this.setupStep6Listeners(stepContainer);
                break;
            default:
                console.warn('⚠️ Unknown step for event listeners:', step);
        }
    }

    /**
     * Mount ConnectorConfigBuilder (inbound) inside wizard step 1.
     * Replaces the old hardcoded sourceConnectivity dropdown + sourceConfigPanel.
     */
    mountInboundConnectorBuilder(container) {
        const builderEl = container.querySelector('#wizardInboundConnectorContainer');
        if (!builderEl) return;

        if (typeof ConnectorConfigBuilder === 'undefined') {
            console.warn('[WizardView] ConnectorConfigBuilder not loaded — falling back to legacy connectivity UI');
            return;
        }

        try {
            // Destroy any previous instance
            if (this.inboundBuilder) {
                try { this.inboundBuilder.destroy(); } catch (_) {}
                this.inboundBuilder = null;
            }

            const existingConfig = this.controller?.model?.data?.sourceConnectorConfig || {};
            this.inboundBuilder = new ConnectorConfigBuilder(builderEl, existingConfig, 'inbound');
            this.inboundBuilder.init();

            // Propagate connector type changes into wizard model for summary/validation
            builderEl.addEventListener('change', () => {
                if (!this.inboundBuilder) return;
                try {
                    const connectorCfg = this.inboundBuilder.getConfig();
                    const connType = connectorCfg.connectorType || '';
                    this.dispatchEvent(new CustomEvent('fieldChange', {
                        detail: {
                            field: 'sourceConnectorConfig',
                            value: { ...connectorCfg }
                        }
                    }));
                    // Keep legacy sourceConnectivity string in sync for validation / summary
                    this.dispatchEvent(new CustomEvent('fieldChange', {
                        detail: { field: 'sourceConnectivity', value: this._connectorTypeToLegacy(connType) }
                    }));
                } catch (err) {
                    console.warn('[WizardView] ConnectorConfigBuilder change handler error:', err);
                }
            });
        } catch (err) {
            console.error('[WizardView] Failed to mount ConnectorConfigBuilder:', err);
            this.inboundBuilder = null;
        }
    }

    /**
     * Mount ConnectorConfigBuilder (outbound) inside wizard step 4 (Target Config).
     * Mirrors mountInboundConnectorBuilder — produces the same config as connector.outbound step.
     */
    mountOutboundConnectorBuilder(container) {
        const builderEl = container.querySelector('#wizardOutboundConnectorContainer');
        if (!builderEl) return;

        if (typeof ConnectorConfigBuilder === 'undefined') {
            console.warn('[WizardView] ConnectorConfigBuilder not loaded — cannot mount outbound builder');
            return;
        }

        try {
            if (this.outboundBuilder) {
                try { this.outboundBuilder.destroy(); } catch (_) {}
                this.outboundBuilder = null;
            }

            const existingConfig = { ...(this.controller?.model?.data?.targetConnectorConfig || {}) };

            // Seed a default connector type so the builder has something selected
            // even when the user never touched the Connection tab.
            if (!existingConfig.connectorType) {
                const legacyTarget = this.controller?.model?.data?.targetConnectivity || 'http';
                const DEFAULT_OUTBOUND = {
                    'http':     'http_outbound',
                    'fhir':     'http_outbound',
                    'tcp':      'tcp_mllp_outbound',
                    'file':     'file_writer',
                    'database': 'postgresql_outbound',
                };
                existingConfig.connectorType = DEFAULT_OUTBOUND[legacyTarget] || 'http_outbound';
            }

            this.outboundBuilder = new ConnectorConfigBuilder(builderEl, existingConfig, 'outbound');
            this.outboundBuilder.init();

            builderEl.addEventListener('change', () => {
                if (!this.outboundBuilder) return;
                try {
                    const connectorCfg = this.outboundBuilder.getConfig();
                    this.dispatchEvent(new CustomEvent('fieldChange', {
                        detail: { field: 'targetConnectorConfig', value: { ...connectorCfg } }
                    }));
                    this.dispatchEvent(new CustomEvent('fieldChange', {
                        detail: { field: 'targetConnectivity', value: this._connectorTypeToLegacy(connectorCfg.connectorType || '') }
                    }));
                } catch (err) {
                    console.warn('[WizardView] Outbound ConnectorConfigBuilder change handler error:', err);
                }
            });
        } catch (err) {
            console.error('[WizardView] Failed to mount outbound ConnectorConfigBuilder:', err);
            this.outboundBuilder = null;
        }
    }

    /** Map a full connector type name (e.g. 'tcp_mllp_inbound') to a legacy shorthand */
    _connectorTypeToLegacy(typeName) {
        if (!typeName) return '';
        if (typeName.includes('mllp') || typeName.includes('tcp')) return 'tcp';
        if (typeName.includes('http') || typeName.includes('fhir') || typeName.includes('rest')) return 'http';
        if (typeName.includes('file') || typeName.includes('sftp') || typeName.includes('ftp')) return 'file';
        if (typeName.includes('database') || typeName.includes('postgresql') ||
            typeName.includes('mysql') || typeName.includes('mongo') ||
            typeName.includes('redis') || typeName.includes('oracle')) return 'database';
        if (typeName.includes('kafka') || typeName.includes('rabbitmq') || typeName.includes('mq')) return 'mq';
        return typeName;
    }

    /**
     * Setup Step 1 event listeners
     */
    setupStep1Listeners(container) {
        // Name input with real-time validation
        const nameInput = container.querySelector('#interfaceName');
        if (nameInput) {
            nameInput.addEventListener('input', (e) => {
                this.dispatchEvent(new CustomEvent('fieldChange', {
                    detail: { field: 'name', value: e.target.value }
                }));
            });
        }

        // Description input with character count
        const descInput = container.querySelector('#interfaceDescription');
        const countDisplay = container.querySelector('#descriptionCount');
        if (descInput && countDisplay) {
            descInput.addEventListener('input', (e) => {
                countDisplay.textContent = e.target.value.length;
                this.dispatchEvent(new CustomEvent('fieldChange', {
                    detail: { field: 'description', value: e.target.value }
                }));
            });
        }

        // OOB template selection
        const templateCards = container.querySelectorAll('.oob-template-card');
        templateCards.forEach(card => {
            card.addEventListener('click', () => {
                templateCards.forEach(c => c.classList.remove('selected'));
                card.classList.add('selected');

                const template = card.getAttribute('data-template');
                this.dispatchEvent(new CustomEvent('templateSelected', {
                    detail: { template }
                }));
            });
        });

        // Source Type selector - re-render panel when changed (for FHIR detection)
        const sourceTypeSelect = container.querySelector('#sourceType');
        if (sourceTypeSelect) {
            sourceTypeSelect.addEventListener('change', (e) => {
                const srcType = e.target.value;
                console.log('🔄 Source type changed:', srcType);
                this.updateSourceConfigPanel(container);

                // Auto-set transformation flow when CDA/CCD source is chosen
                const flowSelectEl = container.querySelector('#transformationFlow');
                if (flowSelectEl) {
                    if (srcType === 'ccda' && flowSelectEl.value !== 'ccd_to_fhir') {
                        flowSelectEl.value = 'ccd_to_fhir';
                        flowSelectEl.dispatchEvent(new Event('change'));
                    } else if (srcType === 'hl7v2' && flowSelectEl.value === 'ccd_to_fhir') {
                        flowSelectEl.value = 'hl7_to_fhir';
                        flowSelectEl.dispatchEvent(new Event('change'));
                    }
                }

                this.dispatchEvent(new CustomEvent('fieldChange', {
                    detail: { field: 'sourceType', value: srcType }
                }));
            });
            console.log('✅ Source type selector listener attached');
        }

        // Source configuration inputs
        const sourceHostInput = container.querySelector('#sourceHost');
        if (sourceHostInput) {
            sourceHostInput.addEventListener('input', (e) => {
                this.dispatchEvent(new CustomEvent('fieldChange', {
                    detail: { field: 'sourceHost', value: e.target.value }
                }));
            });
        }

        const sourcePortInput = container.querySelector('#sourcePort');
        if (sourcePortInput) {
            sourcePortInput.addEventListener('input', (e) => {
                this.dispatchEvent(new CustomEvent('fieldChange', {
                    detail: { field: 'sourcePort', value: parseInt(e.target.value) || 0 }
                }));
            });
        }

        // Mount ConnectorConfigBuilder for source (inbound) connectivity
        // This replaces the old hardcoded sourceConnectivity dropdown + sourceConfigPanel
        this.mountInboundConnectorBuilder(container);

        // Transformation Flow selector
        const transformationFlowSelect = container.querySelector('#transformationFlow');
        const flowDescriptionDiv = container.querySelector('#flowDescription');
        const flowDescTitle = container.querySelector('#flowDescTitle');
        const flowDescText = container.querySelector('#flowDescText');

        if (transformationFlowSelect) {
            transformationFlowSelect.addEventListener('change', (e) => {
                const flow = e.target.value;
                console.log('🔄 Transformation flow selected:', flow);

                // Show/hide flow description
                if (flow && flowDescriptionDiv) {
                    const flowDescriptions = {
                        'hl7_to_fhir': {
                            title: '🔀 HL7 v2.x → FHIR R4 Transformation',
                            text: 'Automatically converts incoming HL7 v2.x messages to FHIR R4 resources and delivers them to your FHIR server. Supports ADT, ORU, ORM, and other message types.'
                        },
                        'hl7_to_fhir_r5': {
                            title: '🔀 HL7 v2.x → FHIR R5 Transformation',
                            text: 'Automatically converts incoming HL7 v2.x messages to FHIR R5 resources. Use for systems adopting the latest FHIR standard — structural changes (e.g. Encounter.class, participant roles) apply.'
                        },
                        'ccd_to_fhir': {
                            title: '🔀 CCD/C-CDA → FHIR R4 Transformation',
                            text: 'Automatically converts C-CDA (Consolidated Clinical Document Architecture) documents to FHIR R4 resources. Ideal for EHR interoperability.'
                        },
                        'passthrough': {
                            title: '📦 Passthrough (No Transformation)',
                            text: 'Stores messages without transformation. Useful for archiving, auditing, or manual processing later.'
                        },
                        'fhir_receiver': {
                            title: '📦 FHIR Receiver (Direct Storage)',
                            text: 'Receives FHIR resources via HTTP and stores them without modification. No automatic forwarding - user-driven processing.'
                        },
                        'file_processor': {
                            title: '📦 File Processor (Batch Processing)',
                            text: 'Processes files in batches. Stores content for manual review and processing. Ideal for bulk data loads.'
                        },
                        'others': {
                            title: '⚙️ Other / Custom Pipeline',
                            text: 'Set up your source and target connectors here. After the wizard completes, you\'ll be taken directly to the Pipeline Builder to configure your custom transformation steps.'
                        }
                    };

                    const desc = flowDescriptions[flow];
                    if (desc) {
                        flowDescTitle.textContent = desc.title;
                        flowDescText.textContent = desc.text;
                        flowDescriptionDiv.style.display = 'block';
                    } else {
                        flowDescriptionDiv.style.display = 'none';
                    }
                }

                // Auto-set Source Format when flow implies a specific format
                const sourceTypeSelectEl = container.querySelector('#sourceType');
                if (sourceTypeSelectEl) {
                    if (flow === 'ccd_to_fhir' && sourceTypeSelectEl.value !== 'ccda') {
                        sourceTypeSelectEl.value = 'ccda';
                        sourceTypeSelectEl.dispatchEvent(new Event('change'));
                    } else if ((flow === 'hl7_to_fhir' || flow === 'hl7_to_fhir_r5') && sourceTypeSelectEl.value === 'ccda') {
                        sourceTypeSelectEl.value = 'hl7v2';
                        sourceTypeSelectEl.dispatchEvent(new Event('change'));
                    }
                }

                // Show/hide family filter based on FlowRegistry (falls back to HL7-only check)
                const familyFilterSection = container.querySelector('#wizardFamilyFilterSection');
                if (familyFilterSection) {
                    const showFilter = typeof getFlow !== 'undefined'
                        ? getFlow(flow).showFamilyFilter
                        : (flow === 'hl7_to_fhir' || flow === 'hl7_to_fhir_r5');
                    familyFilterSection.style.display = showFilter ? 'block' : 'none';
                }

                // Re-render target config panel to show/hide delivery mode
                this.updateTargetConfigPanel(container);

                this.dispatchEvent(new CustomEvent('fieldChange', {
                    detail: { field: 'transformationFlow', value: flow }
                }));
            });
            console.log('✅ Transformation flow selector listener attached');
        }

        // Target Connectivity selector - re-render panel when changed
        const targetConnectivitySelect = container.querySelector('#targetConnectivity');
        if (targetConnectivitySelect) {
            targetConnectivitySelect.addEventListener('change', (e) => {
                console.log('🔄 Target connectivity changed:', e.target.value);
                this.updateTargetConfigPanel(container);

                this.dispatchEvent(new CustomEvent('fieldChange', {
                    detail: { field: 'targetConnectivity', value: e.target.value }
                }));
            });
            console.log('✅ Target connectivity selector listener attached');
        }

        // Setup all target config listeners (delivery mode, auth, etc.)
        // This is now handled by the centralized setupTargetConfigListeners method
        this.setupTargetConfigListeners(container);

        // Deployment Mode selector - show/hide delay panel
        const deploymentModeSelect = container.querySelector('#deploymentMode');
        const delaySettingsPanel = container.querySelector('#delaySettingsPanel');
        if (deploymentModeSelect && delaySettingsPanel) {
            deploymentModeSelect.addEventListener('change', (e) => {
                const mode = e.target.value;
                console.log('🚀 Deployment mode changed:', mode);

                // Show delay settings only for 'delayed' mode
                delaySettingsPanel.style.display = mode === 'delayed' ? 'block' : 'none';

                this.dispatchEvent(new CustomEvent('fieldChange', {
                    detail: { field: 'deploymentMode', value: mode }
                }));
            });
            console.log('✅ Deployment mode selector listener attached');
        }

        // Deployment delay input
        const deploymentDelayInput = container.querySelector('#deploymentDelay');
        if (deploymentDelayInput) {
            deploymentDelayInput.addEventListener('input', (e) => {
                this.dispatchEvent(new CustomEvent('fieldChange', {
                    detail: { field: 'deploymentDelay', value: parseInt(e.target.value) || 0 }
                }));
            });
        }

        // Auto-start checkbox
        const autoStartCheckbox = container.querySelector('#autoStart');
        if (autoStartCheckbox) {
            autoStartCheckbox.addEventListener('change', (e) => {
                this.dispatchEvent(new CustomEvent('fieldChange', {
                    detail: { field: 'autoStart', value: e.target.checked }
                }));
            });
        }

        // Logging configuration listeners
        const debugLoggingCheckbox = container.querySelector('#debugLogging');
        if (debugLoggingCheckbox) {
            debugLoggingCheckbox.addEventListener('change', (e) => {
                this.dispatchEvent(new CustomEvent('fieldChange', {
                    detail: { field: 'debugLogging', value: e.target.checked }
                }));
            });
        }

        const logRetentionInput = container.querySelector('#logRetentionDays');
        const retentionSummary = container.querySelector('#retentionSummary');
        if (logRetentionInput) {
            logRetentionInput.addEventListener('input', (e) => {
                const days = parseInt(e.target.value) || 30;
                // Update summary text dynamically
                if (retentionSummary) {
                    retentionSummary.textContent = `${days} days`;
                }
                this.dispatchEvent(new CustomEvent('fieldChange', {
                    detail: { field: 'logRetentionDays', value: days }
                }));
            });
        }

        const retainErrorLogsCheckbox = container.querySelector('#retainErrorLogs');
        const errorRetentionSummary = container.querySelector('#errorRetentionSummary');
        if (retainErrorLogsCheckbox) {
            retainErrorLogsCheckbox.addEventListener('change', (e) => {
                // Update summary text dynamically
                if (errorRetentionSummary) {
                    errorRetentionSummary.textContent = e.target.checked ? 'Kept forever' : 'Same as debug logs';
                    errorRetentionSummary.style.color = e.target.checked ? '#059669' : '#6b7280';
                }
                this.dispatchEvent(new CustomEvent('fieldChange', {
                    detail: { field: 'retainErrorLogs', value: e.target.checked }
                }));
            });
        }
    }

    /**
     * Setup Step 2 event listeners
     */
    setupStep2Listeners(container) {
        console.log('🔧 Setting up Step 2 listeners (HL7 Parsing)...');

        // HL7 message textarea
        const hl7Message = container.querySelector('#hl7Message');
        if (hl7Message) {
            hl7Message.addEventListener('input', (e) => {
                this.dispatchEvent(new CustomEvent('fieldChange', {
                    detail: { field: 'hl7Message', value: e.target.value }
                }));
            });
            console.log('✅ HL7 message textarea listener attached');
        } else {
            console.warn('⚠️ HL7 message textarea not found');
        }

        // Parse HL7 button
        const parseBtn = container.querySelector('#parseHL7Btn');
        console.log('🔍 Looking for Parse HL7 button...', {
            found: !!parseBtn,
            containerId: container.id,
            containerClass: container.className
        });

        if (parseBtn) {
            parseBtn.addEventListener('click', () => {
                console.log('🔍 Parse HL7 button clicked - dispatching event');
                // Get fresh reference to textarea to avoid stale reference issues
                const currentHL7Message = container.querySelector('#hl7Message');
                const messageContent = currentHL7Message?.value || '';
                console.log('📝 HL7 message content length:', messageContent.length);

                const decodeEscapesChk = container.querySelector('#decodeEscapes');
                const escapeHandling = decodeEscapesChk?.checked ? 'decode' : 'passthrough';
                this.dispatchEvent(new CustomEvent('parseHL7Requested', {
                    detail: { message: messageContent, escapeHandling }
                }));
                console.log('📤 parseHL7Requested event dispatched');
            });
            console.log('✅ Parse HL7 button listener attached');
        } else {
            console.warn('⚠️ Parse HL7 button not found in container');
            // Debug: log all buttons in container
            const allButtons = container.querySelectorAll('button');
            console.log('🔍 All buttons in container:', Array.from(allButtons).map(btn => ({
                id: btn.id,
                className: btn.className,
                textContent: btn.textContent.trim()
            })));
        }

        // Sample HL7 buttons
        const sampleButtons = container.querySelectorAll('.sample-btn');
        sampleButtons.forEach(btn => {
            btn.addEventListener('click', () => {
                const messageType = btn.getAttribute('data-message-type');
                this.dispatchEvent(new CustomEvent('sampleHL7Requested', {
                    detail: { messageType }
                }));
            });
        });

        // Segment toggle functionality
        this.setupSegmentToggles(container);
    }

    /**
     * Setup Step 3 event listeners
     */
    setupStep3Listeners(container) {
        console.log('🔧 Setting up Step 3 FHIR mapping listeners...');

        // Mapping view tabs (By Resource, All Mappings, Issues Only)
        const viewButtons = container.querySelectorAll('.mapping-view-btn');
        viewButtons.forEach(btn => {
            btn.addEventListener('click', (e) => {
                const view = e.target.getAttribute('data-view');
                console.log('📋 Switching mapping view to:', view);

                // Update active state
                viewButtons.forEach(b => {
                    b.style.background = 'white';
                    b.style.color = '#6b7280';
                    b.classList.remove('active');
                });
                e.target.style.background = '#1e3a8a';
                e.target.style.color = 'white';
                e.target.classList.add('active');

                // Implement view filtering
                window.toggleMappingView(view);
            });
        });

        // Resource filter dropdown
        const resourceFilter = container.querySelector('#resource-filter');
        if (resourceFilter) {
            resourceFilter.addEventListener('change', (e) => {
                const filter = e.target.value;
                console.log('🔍 Filtering resources:', filter);

                // Cover both populated groups (resource-mappings-*) and empty placeholders (resource-group-empty-*)
                const populated = container.querySelectorAll('[id^="resource-mappings-"]');
                populated.forEach(group => {
                    const parent = group.closest('[style*="border: 2px"]');
                    if (parent) {
                        const resourceType = group.id.replace('resource-mappings-', '');
                        parent.style.display = (filter === 'all' || resourceType === filter) ? 'block' : 'none';
                    }
                });
                const empty = container.querySelectorAll('[id^="resource-group-empty-"]');
                empty.forEach(el => {
                    const resourceType = el.id.replace('resource-group-empty-', '');
                    el.style.display = (filter === 'all' || resourceType === filter) ? 'block' : 'none';
                });
            });
        }

        // Advanced options toggle
        const advancedToggle = container.querySelector('#toggle-advanced-options');
        const advancedPanel = container.querySelector('#advanced-options-panel');
        if (advancedToggle && advancedPanel) {
            advancedToggle.addEventListener('click', () => {
                const isHidden = advancedPanel.style.display === 'none' || !advancedPanel.style.display;
                advancedPanel.style.display = isHidden ? 'block' : 'none';
                const icon = container.querySelector('#advanced-toggle-icon');
                if (icon) icon.textContent = isHidden ? '▲' : '▼';
            });
        }
    }

    /**
     * Setup Step 4 event listeners (HL7 Parsing & Sample)
     */
    setupStep4Listeners(container) {
        console.log('🔧 Setting up Step 4 listeners...');

        // HL7 message textarea
        const hl7Message = container.querySelector('#hl7Message');
        if (hl7Message) {
            hl7Message.addEventListener('input', (e) => {
                this.dispatchEvent(new CustomEvent('fieldChange', {
                    detail: { field: 'hl7Message', value: e.target.value }
                }));
            });
            console.log('✅ HL7 message textarea listener attached');
        } else {
            console.warn('⚠️ HL7 message textarea not found');
        }

        // Parse HL7 button
        const parseBtn = container.querySelector('#parseHL7Btn');
        console.log('🔍 Looking for Parse HL7 button...', {
            found: !!parseBtn,
            containerId: container.id,
            containerClass: container.className
        });

        if (parseBtn) {
            parseBtn.addEventListener('click', () => {
                console.log('🔍 Parse HL7 button clicked - dispatching event');
                // Get fresh reference to textarea to avoid stale reference issues
                const currentHL7Message = container.querySelector('#hl7Message');
                const messageContent = currentHL7Message?.value || '';
                console.log('📝 HL7 message content length:', messageContent.length);

                const decodeEscapesChk = container.querySelector('#decodeEscapes');
                const escapeHandling = decodeEscapesChk?.checked ? 'decode' : 'passthrough';
                this.dispatchEvent(new CustomEvent('parseHL7Requested', {
                    detail: { message: messageContent, escapeHandling }
                }));
                console.log('📤 parseHL7Requested event dispatched');
            });
            console.log('✅ Parse HL7 button listener attached');
        } else {
            console.warn('⚠️ Parse HL7 button not found in container');
            // Debug: log all buttons in container
            const allButtons = container.querySelectorAll('button');
            console.log('🔍 All buttons in container:', Array.from(allButtons).map(btn => ({
                id: btn.id,
                className: btn.className,
                textContent: btn.textContent.trim()
            })));
        }

        // Sample HL7 buttons
        const sampleButtons = container.querySelectorAll('.sample-btn');
        sampleButtons.forEach(btn => {
            btn.addEventListener('click', () => {
                const messageType = btn.getAttribute('data-message-type');
                this.dispatchEvent(new CustomEvent('sampleHL7Requested', {
                    detail: { messageType }
                }));
            });
        });

        // Segment toggle functionality
        this.setupSegmentToggles(container);

        // Mount the outbound ConnectorConfigBuilder — same component as pipeline builder
        this.mountOutboundConnectorBuilder(container);

        console.log('✅ Step 4 outbound connector builder mounted');
    }

    /**
     * Setup segment toggle functionality
     */
    setupSegmentToggles(container) {
        // Setup click handlers for segment headers
        const segmentHeaders = container.querySelectorAll('.segment-header');
        segmentHeaders.forEach(header => {
            header.addEventListener('click', (e) => {
                const segmentCard = header.closest('.segment-card');
                const segmentContent = segmentCard.querySelector('.segment-content');
                const expandIcon = header.querySelector('.expand-icon');

                if (segmentContent.style.display === 'none') {
                    segmentContent.style.display = 'block';
                    expandIcon.textContent = '▲';
                    segmentCard.classList.add('expanded');
                } else {
                    segmentContent.style.display = 'none';
                    expandIcon.textContent = '▼';
                    segmentCard.classList.remove('expanded');
                }
            });
        });

        // Setup action buttons in transformation summary
        const expandAllBtn = container.querySelector('.action-btn');
        if (expandAllBtn && expandAllBtn.textContent.includes('Expand All')) {
            expandAllBtn.addEventListener('click', () => {
                this.expandAllSegments(container);
            });
        }
    }

    /**
     * Expand all segments
     */
    expandAllSegments(container = document) {
        const segmentCards = container.querySelectorAll('.segment-card');
        segmentCards.forEach(card => {
            const content = card.querySelector('.segment-content');
            const expandIcon = card.querySelector('.expand-icon');
            if (content && expandIcon) {
                content.style.display = 'block';
                expandIcon.textContent = '▲';
                card.classList.add('expanded');
            }
        });
    }

    /**
     * Collapse all segments
     */
    collapseAllSegments(container = document) {
        const segmentCards = container.querySelectorAll('.segment-card');
        segmentCards.forEach(card => {
            const content = card.querySelector('.segment-content');
            const expandIcon = card.querySelector('.expand-icon');
            if (content && expandIcon) {
                content.style.display = 'none';
                expandIcon.textContent = '▼';
                card.classList.remove('expanded');
            }
        });
    }

    /**
     * Setup Step 5 event listeners (Complex mapping interface)
     */
    setupStep5Listeners(container) {
        // Message type change
        const messageType = container.querySelector('#messageType');
        if (messageType) {
            messageType.addEventListener('change', (e) => {
                this.dispatchEvent(new CustomEvent('fieldChange', {
                    detail: { field: 'messageType', value: e.target.value }
                }));
            });
        }

        // Template selection
        const templateRadios = container.querySelectorAll('input[name="mappingTemplate"]');
        templateRadios.forEach(radio => {
            radio.addEventListener('change', (e) => {
                if (e.target.checked) {
                    this.dispatchEvent(new CustomEvent('templateSelected', {
                        detail: { templateId: e.target.value }
                    }));
                }
            });
        });

        // Mapping tabs
        const mappingTabs = container.querySelectorAll('.mapping-tab');
        const mappingPanels = container.querySelectorAll('.mapping-panel');

        mappingTabs.forEach(tab => {
            tab.addEventListener('click', () => {
                const targetTab = tab.getAttribute('data-tab');

                // Update tabs
                mappingTabs.forEach(t => t.classList.remove('active'));
                tab.classList.add('active');

                // Update panels
                mappingPanels.forEach(p => p.classList.remove('active'));
                const targetPanel = container.querySelector(`#${targetTab}Mapping`);
                if (targetPanel) {
                    targetPanel.classList.add('active');
                }
            });
        });

        // Add mapping button (table view)
        const addMappingBtn = container.querySelector('#addMappingRow');
        if (addMappingBtn) {
            addMappingBtn.addEventListener('click', () => {
                this.addMappingRow(container);
            });
        }

        // Validate mappings
        const validateBtn = container.querySelector('#validateMappings');
        if (validateBtn) {
            validateBtn.addEventListener('click', () => {
                this.dispatchEvent(new CustomEvent('validateMappings'));
            });
        }

        // Test transform
        const testBtn = container.querySelector('#testTransform');
        if (testBtn) {
            testBtn.addEventListener('click', () => {
                this.dispatchEvent(new CustomEvent('testTransform'));
            });
        }
    }

    /**
     * Setup Step 6 event listeners
     */
    setupStep6Listeners(container) {
        // Export configuration
        const exportBtn = container.querySelector('#exportConfig');
        if (exportBtn) {
            exportBtn.addEventListener('click', () => {
                this.dispatchEvent(new CustomEvent('exportConfiguration'));
            });
        }

        // Final test
        const testBtn = container.querySelector('#testFullTransform');
        if (testBtn) {
            testBtn.addEventListener('click', () => {
                this.dispatchEvent(new CustomEvent('finalTest'));
            });
        }
    }

    /**
     * Add new mapping row to table
     */
    addMappingRow(container) {
        const tableBody = container.querySelector('#mappingTableBody');
        if (!tableBody) return;

        // Remove empty row if exists
        const emptyRow = tableBody.querySelector('.empty-row');
        if (emptyRow) {
            emptyRow.remove();
        }

        const newRow = document.createElement('tr');
        newRow.innerHTML = `
            <td>
                <input type="text" class="form-control" placeholder="e.g., PID.5.1" data-field="hl7Path">
            </td>
            <td>
                <input type="text" class="form-control" placeholder="e.g., Patient.name[0].family" data-field="fhirPath">
            </td>
            <td>
                <div class="form-check">
                    <input type="checkbox" class="form-check-input" data-field="required">
                </div>
            </td>
            <td>
                <select class="form-control" data-field="transform">
                    <option value="direct">Direct</option>
                    <option value="uppercase">Uppercase</option>
                    <option value="lowercase">Lowercase</option>
                    <option value="date">Date Format</option>
                    <option value="custom">Custom</option>
                </select>
            </td>
            <td>
                <button type="button" class="btn btn-sm btn-outline-danger"
                        onclick="this.closest('tr').remove()" title="Delete">
                    <span>🗑️</span>
                </button>
            </td>
        `;

        tableBody.appendChild(newRow);

        // Focus on the first input
        const firstInput = newRow.querySelector('input[data-field="hl7Path"]');
        if (firstInput) {
            firstInput.focus();
        }
    }

    /**
     * Attach global event listeners
     */
    attachEventListeners() {
        // Use event delegation for better performance and dynamic content
        this.container.addEventListener('click', (e) => {
            console.log('🖱️ Click event detected on:', e.target);

            // Modal controls
            if (e.target.closest('#wizardClose')) {
                console.log('🚪 Close button clicked');
                this.closeModal();
            } else if (e.target.closest('#wizardMaximize')) {
                console.log('⛶ Maximize button clicked');
                this.toggleMaximize();
            }
            // Navigation buttons
            else if (e.target.closest('#wizardPrevious')) {
                this.dispatchEvent(new CustomEvent('previousStep'));
            } else if (e.target.closest('#wizardNext')) {
                console.log('➡️ Next button clicked');
                console.log('📤 Dispatching nextStep event from view');
                const event = new CustomEvent('nextStep');
                console.log('🎯 Event created:', event);
                this.dispatchEvent(event);
                console.log('📨 Event dispatched');
            } else if (e.target.closest('#wizardFinish')) {
                this.dispatchEvent(new CustomEvent('finish'));
            }
            // Template selection
            else if (e.target.closest('.oob-template-item')) {
                const templateItem = e.target.closest('.oob-template-item');
                const templateId = templateItem.dataset.templateId;
                this.selectTemplate(templateId);
            }
            // Step navigation
            else if (e.target.closest('.wizard-step') || e.target.closest('.wizard-step-creative')) {
                const stepElement = e.target.closest('.wizard-step') || e.target.closest('.wizard-step-creative');
                const step = parseInt(stepElement.dataset.step);
                console.log('🎯 Step navigation clicked:', step);
                this.dispatchEvent(new CustomEvent('goToStep', {
                    detail: { step }
                }));
            }
        });

        // Form input changes for validation
        this.container.addEventListener('input', (e) => {
            if (e.target.matches('input, select, textarea')) {
                console.log('📝 Input changed, running validation:', e.target.id, e.target.value);

                // IMPORTANT: Don't check for duplicates on every keystroke
                // Duplicate check happens on blur event (when user leaves field)
                // or when clicking Next button
                if (e.target.id === 'interfaceName') {
                    console.log('ℹ️ Name input detected, but NOT checking duplicates (will check on blur)');
                }

                // Sync form data to model
                this.syncFormDataToModel();

                this.validateCurrentStep();
            }
        });

        // Check for duplicate name when user leaves the field
        this.container.addEventListener('blur', (e) => {
            if (e.target.id === 'interfaceName') {
                const name = e.target.value?.trim();
                if (name && name.length >= 3) {
                    console.log('📝 Name field blur, checking for duplicates...');
                    this.checkDuplicateName(name);
                }
            }
        }, true); // Use capture phase to catch blur events

        // Template radio button changes
        this.container.addEventListener('change', (e) => {
            if (e.target.matches('input[name="mappingTemplate"]')) {
                this.selectTemplate(e.target.value);
            }
        });

        // Keyboard navigation
        this.container.addEventListener('keydown', (e) => {
            if (e.key === 'Escape') {
                this.closeModal();
            }
        });
    }

    /**
     * Select a mapping template
     */
    selectTemplate(templateId) {
        console.log('🎯 Selecting template:', templateId);

        // Update visual selection
        const templates = this.container.querySelectorAll('.oob-template-item');
        templates.forEach(item => {
            item.classList.remove('selected');
            const radio = item.querySelector('input[type="radio"]');
            if (radio) radio.checked = false;
        });

        const selectedTemplate = this.container.querySelector(`[data-template-id="${templateId}"]`);
        if (selectedTemplate) {
            selectedTemplate.classList.add('selected');
            const radio = selectedTemplate.querySelector('input[type="radio"]');
            if (radio) radio.checked = true;
        }

        // Notify controller of template selection
        this.dispatchEvent(new CustomEvent('templateSelected', {
            detail: { templateId }
        }));

        // Re-validate the current step
        this.validateCurrentStep();
    }

    /**
     * Open modal (show method)
     */
    openModal() {
        this.show();
    }

    /**
     * Close modal (hide method)
     */
    closeModal() {
        this.hide();
    }

    /**
     * Show the wizard modal
     */
    show() {
        if (this.modal) {
            this.modal.classList.add('show');
            this.modal.style.display = 'flex';

            // Focus the first focusable element
            setTimeout(() => {
                const firstFocusable = this.modal.querySelector('button:not([disabled]), input, select, textarea');
                if (firstFocusable) {
                    firstFocusable.focus();
                }
            }, 100);

            // Prevent body scrolling
            document.body.style.overflow = 'hidden';
        }
    }

    /**
     * Hide the wizard modal
     */
    hide() {
        if (this.modal) {
            this.modal.classList.remove('show', 'maximized');
            this.modal.style.display = 'none';
            document.body.style.overflow = '';
        }
    }

    /**
     * Validate current step and update navigation
     */
    validateCurrentStep() {
        const currentStep = this.getCurrentStepNumber();
        console.log('🔍 Validating step:', currentStep);
        let isValid = false;

        switch (currentStep) {
            case 1:
                isValid = this.validateStep1();
                break;
            case 2:
                isValid = this.validateStep2();
                break;
            case 3:
                isValid = this.validateStep3();
                break;
            case 4:
                isValid = this.validateStep4();
                break;
            case 5:
                isValid = this.validateStep5();
                break;
            case 6:
                isValid = true; // Summary step is always valid
                break;
        }

        console.log('✅ Step', currentStep, 'validation result:', isValid);
        this.updateNavigationButtons(currentStep, isValid);
        return isValid;
    }

    /**
     * Sync form data to model
     * Uses InterfaceConfigComponents for unified data collection (same as Edit modal)
     */
    syncFormDataToModel() {
        console.log('🔄 Syncing form data to model (using InterfaceConfigComponents)...');

        // Use the unified InterfaceConfigComponents for data collection
        // This ensures Wizard and Edit modal use the same logic
        // Wizard uses empty prefix '', Edit modal uses 'edit'
        const configData = InterfaceConfigComponents.collectFormData(this.container, '');

        // Merge basic info fields with config data from shared component
        // configData already includes logging settings via InterfaceConfigComponents.collectFormData()
        const data = {
            name: this.container.querySelector('#interfaceName')?.value || '',
            description: this.container.querySelector('#interfaceDescription')?.value || '',
            transformationFlow: this.container.querySelector('#transformationFlow')?.value,
            ...configData  // Includes sourceType, sourceConfig, targetConfig, deployment settings, AND logging settings
        };

        // Get target type if available
        const targetType = this.container.querySelector('#targetType')?.value;
        if (targetType) data.targetType = targetType;

        // Collect accepted message families from wizard chip picker
        const familiesInput = this.container.querySelector('#wizardAcceptedMessageFamilies');
        if (familiesInput) {
            try {
                data.acceptedMessageFamilies = familiesInput.value ? JSON.parse(familiesInput.value) : null;
            } catch (e) {
                data.acceptedMessageFamilies = null;
            }
        }

        // Collect inbound connector config from ConnectorConfigBuilder if available
        if (this.inboundBuilder) {
            try {
                const connectorCfg = this.inboundBuilder.getConfig();
                data.sourceConnectorConfig = { ...connectorCfg };
                data.sourceConnectivity = this._connectorTypeToLegacy(connectorCfg.connectorType || '');
                // Merge low-level host/port into sourceConfig for backward compat
                if (connectorCfg.config) {
                    data.sourceConfig = { ...data.sourceConfig, ...connectorCfg.config };
                }
            } catch (err) {
                console.warn('[WizardView] Error collecting inbound connector config:', err);
            }
        }

        // Collect outbound connector config from ConnectorConfigBuilder if available
        if (this.outboundBuilder) {
            try {
                const connectorCfg = this.outboundBuilder.getConfig();
                data.targetConnectorConfig = { ...connectorCfg };
                data.targetConnectivity = this._connectorTypeToLegacy(connectorCfg.connectorType || '');
                if (connectorCfg.config) {
                    data.targetConfig = { ...data.targetConfig, ...connectorCfg.config };
                }
            } catch (err) {
                console.warn('[WizardView] Error collecting outbound connector config:', err);
            }
        }

        console.log('📝 Synced form values via InterfaceConfigComponents:', data);

        // Update model data (dispatch event for async handling)
        if (Object.keys(data).length > 0) {
            this.dispatchEvent(new CustomEvent('dataSync', {
                detail: data,
                bubbles: true,
                composed: true
            }));
        }

        return data;
    }

    /**
     * Legacy form extraction - fallback when FormBindingEngine not available
     * This maintains backward compatibility during migration
     */
    extractFormDataLegacy(sourceConnectivity, targetConnectivity) {
        const data = {};

        // Basic Info
        const interfaceName = this.container.querySelector('#interfaceName')?.value;
        const interfaceDescription = this.container.querySelector('#interfaceDescription')?.value;
        if (interfaceName !== undefined) data.name = interfaceName || '';
        if (interfaceDescription !== undefined) data.description = interfaceDescription || '';

        // Source Configuration
        const sourceType = this.container.querySelector('#sourceType')?.value;
        const transformationFlow = this.container.querySelector('#transformationFlow')?.value;

        if (sourceType !== undefined) data.sourceType = sourceType;
        if (transformationFlow !== undefined) data.transformationFlow = transformationFlow;

        // Use ConnectorConfigBuilder output when available
        if (this.inboundBuilder) {
            this.inboundBuilder.collectConfig();
            data.sourceConnectorConfig = { ...this.inboundBuilder.config };
            const ct = this.inboundBuilder.config.connectorType || '';
            data.sourceConnectivity = this._connectorTypeToLegacy(ct);
            if (this.inboundBuilder.config.config) {
                data.sourceConfig = { ...this.inboundBuilder.config.config };
            }
        }
        if (this.outboundBuilder) {
            this.outboundBuilder.collectConfig();
            data.targetConnectorConfig = { ...this.outboundBuilder.config };
            const ct = this.outboundBuilder.config.connectorType || '';
            data.targetConnectivity = this._connectorTypeToLegacy(ct);
            if (this.outboundBuilder.config.config) {
                data.targetConfig = { ...data.targetConfig, ...this.outboundBuilder.config.config };
            }
        }
        if (!this.inboundBuilder) {
            // Legacy fallback
            const sourcePort = this.container.querySelector('#sourcePort')?.value;
            const sourceHost = this.container.querySelector('#sourceHost')?.value;
            if (sourceConnectivity !== undefined) data.sourceConnectivity = sourceConnectivity;
            if (sourcePort || sourceHost) {
                data.sourceConfig = data.sourceConfig || {};
                if (sourcePort) data.sourceConfig.port = parseInt(sourcePort);
                if (sourceHost) data.sourceConfig.host = sourceHost;
            }
        }

        // Database Connectivity (legacy path only, ConnectorConfigBuilder handles this natively)
        if (!this.inboundBuilder && sourceConnectivity === 'database') {
            data.sourceConfig = data.sourceConfig || {};
            const fields = ['sourceDbType:db_type', 'sourceHost:host', 'sourcePort:port', 'sourceDatabase:database',
                           'sourceUsername:username', 'sourcePassword:password', 'sourceSslMode:ssl_mode',
                           'sourceTableName:table_name', 'sourceQuery:query', 'sourceIncrementalColumn:incremental_column',
                           'sourceIncrementalType:incremental_type', 'sourcePollingInterval:polling_interval',
                           'sourceMaxRecords:max_records', 'sourceAfterProcessing:after_processing'];

            fields.forEach(mapping => {
                const [id, key] = mapping.split(':');
                const value = this.container.querySelector(`#${id}`)?.value;
                if (value) {
                    data.sourceConfig[key] = ['port', 'polling_interval', 'max_records'].includes(key)
                        ? parseInt(value) : value;
                }
            });
        }

        // Target Configuration
        const targetType = this.container.querySelector('#targetType')?.value;
        const targetEndpoint = this.container.querySelector('#targetEndpoint')?.value;
        if (targetType !== undefined) data.targetType = targetType;
        if (targetConnectivity !== undefined) data.targetConnectivity = targetConnectivity;

        if (targetEndpoint !== undefined) {
            data.targetConfig = data.targetConfig || {};
            data.targetConfig.endpoint = targetEndpoint;

            // Delivery mode
            const deliveryModeIndividual = this.container.querySelector('#deliveryModeIndividual');
            if (deliveryModeIndividual !== null) {
                data.targetConfig.deliveryMode = deliveryModeIndividual?.checked ? 'individual' : 'bundle';
            }

            // Authentication
            const authType = this.container.querySelector('#authType')?.value;
            if (authType && authType !== 'none') {
                data.targetConfig.authType = authType;
                if (authType === 'basic') {
                    data.targetConfig.username = this.container.querySelector('#authUsername')?.value;
                    data.targetConfig.password = this.container.querySelector('#authPassword')?.value;
                } else if (authType === 'bearer') {
                    data.targetConfig.bearerToken = this.container.querySelector('#authToken')?.value;
                } else if (authType === 'api_key') {
                    data.targetConfig.apiKey = this.container.querySelector('#authApiKey')?.value;
                    data.targetConfig.apiKeyHeader = this.container.querySelector('#authApiKeyHeader')?.value || 'X-API-Key';
                }
            }

            // Version and format
            const targetVersion = this.container.querySelector('#targetVersion')?.value;
            const targetFormat = this.container.querySelector('#targetFormat')?.value;
            if (targetVersion) data.targetConfig.version = targetVersion;
            if (targetFormat) data.targetConfig.format = targetFormat;
        }

        return data;
    }

    /**
     * Get current step number
     */
    getCurrentStepNumber() {
        // Try creative steps first
        let activeStep = this.container.querySelector('.wizard-step-creative.active');
        if (!activeStep) {
            activeStep = this.container.querySelector('.wizard-step.active');
        }

        if (activeStep && activeStep.dataset.step) {
            return parseInt(activeStep.dataset.step);
        }

        // Fallback: check the step content container
        const stepContent = this.container.querySelector('.wizard-step-content[data-step]');
        if (stepContent && stepContent.dataset.step) {
            return parseInt(stepContent.dataset.step);
        }

        return 1; // Default to step 1
    }

    /**
     * Validate step 1: Basic info
     */
    validateStep1() {
        const nameInput = this.container.querySelector('#interfaceName');
        const name = nameInput?.value?.trim();

        const hasValidName = name && name.length >= 3;
        const noDuplicate = !this.isDuplicateName;

        const flowSelect = this.container.querySelector('#transformationFlow');
        const hasFlow = !!(flowSelect?.value);

        return hasValidName && noDuplicate && hasFlow;
    }

    /**
     * Check for duplicate interface name
     */
    async checkDuplicateName(name) {
        // Don't check if name is too short
        if (!name || name.length < 3) {
            this.clearNameError();
            this.isDuplicateName = false;
            return;
        }

        // Don't re-check if name hasn't changed
        if (name === this.lastCheckedName) {
            return;
        }

        try {
            // Show checking indicator
            this.showNameChecking();

            console.log('🔍 Checking duplicate name:', name);
            this.lastCheckedName = name;

            const response = await fetch('/api/wizard/check-duplicate', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({ name: name })
            });

            const result = await response.json();

            if (result.success && result.isDuplicate) {
                console.log('⚠️ Duplicate name detected:', name);
                this.isDuplicateName = true;
                this.showNameError(result.message);
            } else {
                console.log('✅ Name is available:', name);
                this.isDuplicateName = false;
                this.showNameSuccess('Name is available');
            }

            // Re-validate the step to update button states
            this.validateCurrentStep();

        } catch (error) {
            console.error('❌ Error checking duplicate name:', error);
            // Don't block on error, just log it
            this.isDuplicateName = false;
            this.clearNameError();
        }
    }

    /**
     * Show checking indicator
     */
    showNameChecking() {
        const nameInput = this.container.querySelector('#interfaceName');
        if (!nameInput) return;

        // Find or create status element
        let statusDiv = nameInput.parentElement.querySelector('.name-status-message');
        if (!statusDiv) {
            statusDiv = document.createElement('div');
            statusDiv.className = 'name-status-message';
            statusDiv.style.cssText = 'font-size: 12px; margin-top: 4px; font-weight: 500;';
            nameInput.parentElement.appendChild(statusDiv);
        }

        statusDiv.style.color = '#6b7280';
        statusDiv.textContent = '🔍 Checking availability...';
    }

    /**
     * Show name success message
     */
    showNameSuccess(message) {
        const nameInput = this.container.querySelector('#interfaceName');
        if (!nameInput) return;

        // Remove error styling
        nameInput.style.borderColor = '#10b981';
        nameInput.style.borderWidth = '2px';

        // Find or create status element
        let statusDiv = nameInput.parentElement.querySelector('.name-status-message');
        if (!statusDiv) {
            statusDiv = document.createElement('div');
            statusDiv.className = 'name-status-message';
            statusDiv.style.cssText = 'font-size: 12px; margin-top: 4px; font-weight: 500;';
            nameInput.parentElement.appendChild(statusDiv);
        }

        statusDiv.style.color = '#10b981';
        statusDiv.textContent = '✅ ' + message;

        // Auto-clear success message after 2 seconds
        setTimeout(() => {
            if (statusDiv && statusDiv.textContent.includes('available')) {
                nameInput.style.borderColor = '';
                nameInput.style.borderWidth = '';
                statusDiv.remove();
            }
        }, 2000);
    }

    /**
     * Show name error message
     */
    showNameError(message) {
        const nameInput = this.container.querySelector('#interfaceName');
        if (!nameInput) return;

        // Add error styling
        nameInput.style.borderColor = '#dc2626';
        nameInput.style.borderWidth = '2px';

        // Find or create status message element
        let statusDiv = nameInput.parentElement.querySelector('.name-status-message');
        if (!statusDiv) {
            statusDiv = document.createElement('div');
            statusDiv.className = 'name-status-message';
            statusDiv.style.cssText = 'font-size: 12px; margin-top: 4px; font-weight: 500;';
            nameInput.parentElement.appendChild(statusDiv);
        }

        statusDiv.style.color = '#dc2626';
        statusDiv.textContent = '⚠️ ' + message;
    }

    /**
     * Clear name error message
     */
    clearNameError() {
        const nameInput = this.container.querySelector('#interfaceName');
        if (!nameInput) return;

        // Remove styling
        nameInput.style.borderColor = '';
        nameInput.style.borderWidth = '';

        // Remove status message
        const statusDiv = nameInput.parentElement.querySelector('.name-status-message');
        if (statusDiv) {
            statusDiv.remove();
        }
    }

    /**
     * Validate step 2: HL7 Parsing & Sample (updated for consolidated steps)
     */
    validateStep2() {
        // Check if HL7 message is provided AND parsed
        const hl7Message = this.container.querySelector('#hl7Message')?.value;
        const parseResults = this.container.querySelector('#parsingResults');
        const isResultsVisible = parseResults && parseResults.style.display !== 'none';

        // Valid if HL7 message is provided AND has been successfully parsed
        const hasHL7Content = hl7Message && hl7Message.trim().length > 0;
        const hasParsedResults = isResultsVisible && parseResults.innerHTML.trim().length > 0;

        const isValid = hasHL7Content && hasParsedResults;
        console.log('🔍 Step 2 validation:', {
            hasHL7Content,
            hasParsedResults,
            isValid
        });

        return isValid;
    }

    /**
     * Validate step 3: FHIR Transform/Mapping
     */
    validateStep3() {
        // Step 3 is valid if we have FHIR transformation results with mappings
        const data = window.wizardController?.getCurrentData();
        const hasFhirResult = data?.fhirTransformResult;
        const hasMappings = data?.fhirTransformResult?.atomicMappings?.length > 0;

        console.log('🔍 Step 3 validation:', { hasFhirResult: !!hasFhirResult, hasMappings, mappingCount: data?.fhirTransformResult?.atomicMappings?.length });

        // Valid if we have FHIR transformation result and mappings
        return hasFhirResult && hasMappings;
    }

    /**
     * Validate step 4: Target Configuration (Final Step)
     */
    validateStep4() {
        // Sink-only flows have no outbound connector to configure — always valid
        const sinkInput = this.container.querySelector('input[type="hidden"]#targetConnectivity');
        if (sinkInput?.value === 'sink') return true;

        // Sync DOM → config then delegate to builder's schema-aware validate()
        if (this.outboundBuilder) {
            try {
                this.outboundBuilder.getConfig(); // syncs DOM fields into this.config
                const result = this.outboundBuilder.validate();
                return result.valid === true;
            } catch {
                return false;
            }
        }

        // Builder not yet mounted — allow proceed
        return true;
    }

    /**
     * Validate step 5: Mapping configuration
     */
    validateStep5() {
        const messageType = this.container.querySelector('#messageType')?.value;
        const selectedTemplate = this.container.querySelector('input[name="mappingTemplate"]:checked')?.value;

        return messageType && selectedTemplate;
    }

    /**
     * Update navigation button states
     */
    updateNavigationButtons(currentStep, isValid) {
        const nextBtn = this.container.querySelector('#wizardNext');
        const finishBtn = this.container.querySelector('#wizardFinish');
        const prevBtn = this.container.querySelector('#wizardPrevious');

        // Previous button
        if (prevBtn) {
            prevBtn.disabled = currentStep === 1;
        }

        // Next button (hide on last step - Step 4)
        if (nextBtn) {
            nextBtn.disabled = !isValid || currentStep === 4;
            nextBtn.style.display = currentStep === 4 ? 'none' : 'inline-flex';
        }

        // Finish button (show on last step - Step 4)
        if (finishBtn) {
            finishBtn.disabled = !isValid;
            finishBtn.style.display = currentStep === 4 ? 'inline-flex' : 'none';
        }
    }

    /**
     * Toggle maximize state
     */
    toggleMaximize() {
        if (this.modal) {
            this.modal.classList.toggle('maximized');

            const maximizeBtn = document.getElementById('wizardMaximize');
            const icon = maximizeBtn?.querySelector('span');
            if (icon) {
                icon.textContent = this.modal.classList.contains('maximized') ? '🗗' : '⛶';
            }
        }
    }

    /**
     * Check if wizard is visible
     */
    isVisible() {
        return this.modal?.classList.contains('show') || false;
    }

    /**
     * Show validation errors
     */
    showValidation(validation) {
        const validationContainer = document.getElementById('wizardValidation');
        if (!validationContainer) return;

        const errors = Object.entries(validation.errors || {});
        const warnings = Object.entries(validation.warnings || {});

        if (errors.length === 0 && warnings.length === 0) {
            validationContainer.innerHTML = '';
            return;
        }

        const errorHtml = errors.map(([field, message]) => `
            <div class="validation-message error">
                <span class="validation-icon">❌</span>
                <span class="validation-text">${message}</span>
            </div>
        `).join('');

        const warningHtml = warnings.map(([field, message]) => `
            <div class="validation-message warning">
                <span class="validation-icon">⚠️</span>
                <span class="validation-text">${message}</span>
            </div>
        `).join('');

        validationContainer.innerHTML = errorHtml + warningHtml;
    }

    /**
     * Show loading state
     */
    showLoading(message = 'Processing...') {
        // Implementation for loading overlay
        const loadingOverlay = document.createElement('div');
        loadingOverlay.className = 'wizard-loading-overlay';
        loadingOverlay.id = 'wizardLoading';
        loadingOverlay.innerHTML = `
            <div class="loading-spinner"></div>
            <div class="loading-message">${message}</div>
        `;

        this.modal?.appendChild(loadingOverlay);
    }

    /**
     * Hide loading state
     */
    hideLoading() {
        const loadingOverlay = document.getElementById('wizardLoading');
        if (loadingOverlay) {
            loadingOverlay.remove();
        }
    }

    /**
     * Utility function for delays
     */
    sleep(ms) {
        return new Promise(resolve => setTimeout(resolve, ms));
    }

    /**
     * Show notification
     */
    showNotification(message, type = 'info', duration = 3000) {
        const notification = document.createElement('div');
        notification.className = `wizard-notification ${type}`;
        notification.innerHTML = `
            <span class="notification-icon">${this.getNotificationIcon(type)}</span>
            <span class="notification-message">${message}</span>
            <button class="notification-close" onclick="this.parentElement.remove()">×</button>
        `;

        // Position at top-right of wizard
        notification.style.cssText = `
            position: absolute;
            top: 20px;
            right: 20px;
            z-index: 10001;
        `;

        this.modal?.appendChild(notification);

        // Auto-remove after duration
        setTimeout(() => notification.remove(), duration);
    }

    /**
     * Get notification icon based on type
     */
    getNotificationIcon(type) {
        const icons = {
            success: '✅',
            error: '❌',
            warning: '⚠️',
            info: 'ℹ️'
        };
        return icons[type] || icons.info;
    }

    /**
     * Show wizard completion modal with action buttons
     */
    showCompletionModal(data) {
        const { interfaceId, name, status, messageType, autoStart } = data;

        // Create completion modal overlay
        const completionModal = document.createElement('div');
        completionModal.className = 'wizard-completion-modal';
        completionModal.innerHTML = `
            <div class="completion-modal-overlay" style="
                position: fixed;
                top: 0;
                left: 0;
                right: 0;
                bottom: 0;
                background: rgba(0, 0, 0, 0.7);
                display: flex;
                align-items: center;
                justify-content: center;
                z-index: 10002;
                animation: fadeIn 0.3s ease;
            ">
                <div class="completion-modal-content" style="
                    background: white;
                    border-radius: 12px;
                    padding: 32px;
                    max-width: 600px;
                    width: 90%;
                    box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
                    animation: slideUp 0.3s ease;
                ">
                    <!-- Success Icon -->
                    <div style="text-align: center; margin-bottom: 24px;">
                        <div style="
                            width: 80px;
                            height: 80px;
                            margin: 0 auto;
                            background: linear-gradient(135deg, #10b981 0%, #059669 100%);
                            border-radius: 50%;
                            display: flex;
                            align-items: center;
                            justify-content: center;
                            font-size: 48px;
                            color: white;
                            box-shadow: 0 8px 24px rgba(16, 185, 129, 0.3);
                        ">✓</div>
                    </div>

                    <!-- Title -->
                    <h2 style="
                        text-align: center;
                        color: #1e3a8a;
                        font-size: 24px;
                        font-weight: 600;
                        margin: 0 0 12px 0;
                    ">Interface Created Successfully!</h2>

                    <!-- Interface Name -->
                    <p style="
                        text-align: center;
                        color: #6b7280;
                        font-size: 16px;
                        margin: 0 0 24px 0;
                    ">"<strong>${name}</strong>" is ready</p>

                    <!-- Status Info -->
                    <div style="
                        background: ${status === 'active' ? '#d1fae5' : '#fef3c7'};
                        border: 2px solid ${status === 'active' ? '#10b981' : '#f59e0b'};
                        border-radius: 8px;
                        padding: 16px;
                        margin-bottom: 24px;
                    ">
                        <div style="display: flex; align-items: center; gap: 12px;">
                            <div style="font-size: 24px;">${status === 'active' ? '🟢' : '📝'}</div>
                            <div>
                                <div style="font-weight: 600; color: #1e3a8a; margin-bottom: 4px;">
                                    ${status === 'active' ? 'Active & Running' : 'Draft - Customize Pipeline'}
                                </div>
                                <div style="font-size: 14px; color: #6b7280;">
                                    ${status === 'active'
                                        ? 'Interface is processing messages'
                                        : 'Configure pipeline steps before activation'}
                                </div>
                            </div>
                        </div>
                    </div>

                    <!-- Action Buttons -->
                    <div style="
                        display: grid;
                        grid-template-columns: 1fr 1fr;
                        gap: 12px;
                        margin-bottom: 16px;
                    ">
                        <button onclick="window.wizardCompletionActions.configurePipeline('${interfaceId}', '${messageType}')" style="
                            background: linear-gradient(135deg, #6366f1 0%, #4f46e5 100%);
                            color: white;
                            border: none;
                            border-radius: 8px;
                            padding: 16px;
                            font-size: 15px;
                            font-weight: 600;
                            cursor: pointer;
                            transition: all 0.2s;
                            box-shadow: 0 4px 12px rgba(99, 102, 241, 0.3);
                        " onmouseover="this.style.transform='translateY(-2px)'; this.style.boxShadow='0 6px 16px rgba(99, 102, 241, 0.4)'"
                           onmouseout="this.style.transform=''; this.style.boxShadow='0 4px 12px rgba(99, 102, 241, 0.3)'">
                            🔀 Configure Pipeline
                        </button>

                        <button onclick="window.wizardCompletionActions.viewMessages('${interfaceId}')" style="
                            background: white;
                            color: #6366f1;
                            border: 2px solid #6366f1;
                            border-radius: 8px;
                            padding: 16px;
                            font-size: 15px;
                            font-weight: 600;
                            cursor: pointer;
                            transition: all 0.2s;
                        " onmouseover="this.style.background='#f5f3ff'"
                           onmouseout="this.style.background='white'">
                            💬 View Messages
                        </button>
                    </div>

                    <!-- Close Button -->
                    <button onclick="window.wizardCompletionActions.close()" style="
                        background: #e5e7eb;
                        color: #374151;
                        border: none;
                        border-radius: 8px;
                        padding: 14px;
                        font-size: 14px;
                        font-weight: 600;
                        cursor: pointer;
                        width: 100%;
                        transition: all 0.2s;
                    " onmouseover="this.style.background='#d1d5db'"
                       onmouseout="this.style.background='#e5e7eb'">
                        Close & Return to Interfaces
                    </button>
                </div>
            </div>
        `;

        // Add animations
        const style = document.createElement('style');
        style.textContent = `
            @keyframes fadeIn {
                from { opacity: 0; }
                to { opacity: 1; }
            }
            @keyframes slideUp {
                from {
                    opacity: 0;
                    transform: translateY(30px);
                }
                to {
                    opacity: 1;
                    transform: translateY(0);
                }
            }
        `;
        document.head.appendChild(style);

        // Add to DOM
        document.body.appendChild(completionModal);

        // Setup action handlers
        window.wizardCompletionActions = {
            configurePipeline: (interfaceId, messageType) => {
                completionModal.remove();
                if (window.wizardController) {
                    window.wizardController.closeWizard();
                }
                // Navigate to pipeline configuration
                if (typeof window.configurePipeline === 'function') {
                    window.configurePipeline(interfaceId, messageType);
                } else {
                    window.location.href = `pipeline-builder.html?interfaceId=${interfaceId}&messageType=${messageType}`;
                }
            },
            viewMessages: (interfaceId) => {
                completionModal.remove();
                if (window.wizardController) {
                    window.wizardController.closeWizard();
                }
                // Navigate to messages page
                if (typeof window.viewInterfaceMessages === 'function') {
                    window.viewInterfaceMessages(interfaceId);
                } else {
                    window.location.href = `messages.html?interfaceId=${interfaceId}`;
                }
            },
            close: () => {
                completionModal.remove();
                if (window.wizardController) {
                    window.wizardController.closeWizard();
                }
            }
        };
    }

    // ====================================
    // BEAUTIFUL HL7 ELEMENT TABLE HELPERS
    // ====================================

    /**
     * Get known HL7 to FHIR mappings
     */
    getKnownHL7ToFHIRMappings() {
        return {
            'MessageHeader': {
                'MSH.3': 'MessageHeader.source.name',
                'MSH.4': 'MessageHeader.source.software',
                'MSH.5': 'MessageHeader.destination[0].name',
                'MSH.6': 'MessageHeader.destination[0].endpoint',
                'MSH.9.1': 'MessageHeader.eventCoding.system',
                'MSH.9.2': 'MessageHeader.eventCoding.code',
                'MSH.9': 'MessageHeader.eventCoding.display',
                'MSH.10': 'MessageHeader.id'
            },
            'Patient': {
                'PID.3': 'Patient.identifier[0].value',
                'PID.5.1': 'Patient.name[0].family',
                'PID.5.2': 'Patient.name[0].given[0]',
                'PID.7': 'Patient.birthDate',
                'PID.8': 'Patient.gender',
                'PID.11.1': 'Patient.address[0].line[0]',
                'PID.11.3': 'Patient.address[0].city',
                'PID.11.4': 'Patient.address[0].state',
                'PID.11.5': 'Patient.address[0].postalCode',
                'PID.13.1': 'Patient.telecom[0].value',
                'PID.13.4': 'Patient.telecom[1].value',
                'PID.18': 'Patient.identifier[1].value'
            },
            'Encounter': {
                'PV1.2': 'Encounter.class.code',
                'PV1.3': 'Encounter.location[0].location.display',
                'PV1.7': 'Encounter.participant[0].individual.display',
                'PV1.19': 'Encounter.identifier[0].value',
                'PV1.44': 'Encounter.period.start'
            }
        };
    }

    /**
     * Get resource type for HL7 field
     */
    getResourceTypeForHL7Field(hl7Field) {
        if (hl7Field.startsWith('MSH.')) return 'MessageHeader';
        if (hl7Field.startsWith('PID.')) return 'Patient';
        if (hl7Field.startsWith('PV1.')) return 'Encounter';
        if (hl7Field.startsWith('EVN.')) return 'MessageHeader';
        if (hl7Field.startsWith('OBX.')) return 'Observation';
        if (hl7Field.startsWith('ORC.')) return 'ServiceRequest';
        if (hl7Field.startsWith('NK1.')) return 'RelatedPerson';
        if (hl7Field.startsWith('AL1.')) return 'AllergyIntolerance';
        if (hl7Field.startsWith('DG1.')) return 'Condition';
        return 'Unknown';
    }

    /**
     * Get FHIR path for HL7 field
     */
    getFHIRPathForHL7Field(hl7Field, knownMappings) {
        const resourceType = this.getResourceTypeForHL7Field(hl7Field);
        const mappings = knownMappings[resourceType];

        if (mappings && mappings[hl7Field]) {
            return mappings[hl7Field];
        }

        // Generate reasonable default FHIR path
        return `${resourceType}.${hl7Field.replace(/\./g, '_')}`;
    }

    /**
     * Get resource types from mappings
     */
    getResourceTypesFromMappings(mappings) {
        const resourceTypes = [...new Set(mappings.map(m => m.resourceType))];

        // If no mappings, return common resource types for display
        if (resourceTypes.length === 0) {
            return ['MessageHeader', 'Patient', 'Encounter'];
        }

        return resourceTypes.sort();
    }

    /**
     * Get resource icon
     */
    getResourceIcon(resourceType) {
        const icons = {
            'Patient': '👤',
            'MessageHeader': '📨',
            'Encounter': '🏥',
            'Observation': '🔬',
            'DiagnosticReport': '📋',
            'Procedure': '💉',
            'Organization': '🏢',
            'Practitioner': '👨‍⚕️',
            'Bundle': '📦',
            'ServiceRequest': '📝',
            'RelatedPerson': '👥',
            'AllergyIntolerance': '⚠️',
            'Condition': '🩺',
            'Unknown': '📄'
        };
        return icons[resourceType] || '📄';
    }

    /**
     * Toggle resource card visibility
     */
    toggleResource(index) {
        const content = document.getElementById(`resource-content-${index}`);
        const icon = document.getElementById(`toggle-icon-${index}`);

        if (content && icon) {
            if (content.style.display === 'none') {
                content.style.display = 'block';
                icon.textContent = '▼';
            } else {
                content.style.display = 'none';
                icon.textContent = '▶';
            }
        }
    }

    /**
     * Expand all resources
     */
    expandAllResources() {
        document.querySelectorAll('[id^="resource-content-"]').forEach(el => {
            el.style.display = 'block';
        });
        document.querySelectorAll('[id^="toggle-icon-"]').forEach(el => {
            el.textContent = '▼';
        });
    }

    /**
     * Collapse all resources
     */
    collapseAllResources() {
        document.querySelectorAll('[id^="resource-content-"]').forEach(el => {
            el.style.display = 'none';
        });
        document.querySelectorAll('[id^="toggle-icon-"]').forEach(el => {
            el.textContent = '▶';
        });
    }

    /**
     * View raw JSON (placeholder)
     */
    viewRawJson() {
        console.log('📄 View Raw JSON clicked - implement JSON viewer modal');
        this.showNotification('JSON viewer feature coming soon!', 'info');
    }

    /**
     * Proceed to transform (placeholder)
     */
    proceedToTransform() {
        console.log('⚡ Proceed to Transform clicked');
        this.showNotification('Transformation feature ready!', 'success');
    }

    /**
     * Toggle segment visibility
     */
    toggleSegment(index) {
        const content = document.getElementById(`segment-content-${index}`);
        const icon = document.getElementById(`segment-toggle-icon-${index}`);

        if (content && icon) {
            if (content.style.display === 'none') {
                content.style.display = 'block';
                icon.textContent = '▼';
            } else {
                content.style.display = 'none';
                icon.textContent = '▶';
            }
        }
    }

    /**
     * Show FHIR transformation (replaces current view with FHIR mapping view)
     */
    showFHIRTransformation() {
        console.log('⚡ Transform to FHIR clicked');

        // Get current step data
        const currentData = window.wizardController?.model?.getCurrentStepData();
        if (!currentData?.parsedHL7Data) {
            this.showNotification('No parsed HL7 data available for transformation', 'error');
            return;
        }

        // Replace the parsing results with FHIR transformation
        const parsingResults = document.getElementById('parsingResults');
        if (parsingResults) {
            // Extract atomic mappings from parsed data for beautiful display
            const atomicMappings = this.extractAtomicMappingsFromParsedData(currentData.parsedHL7Data);
            const resourceTypes = this.getResourceTypesFromMappings(atomicMappings);
            const messageType = currentData.detectedMessageType || 'Unknown';
            const segments = currentData.parsedHL7Data?.segments ? Object.keys(currentData.parsedHL7Data.segments).length : 0;
            const fields = currentData.parsedHL7Data?.enhancedSegments ?
                Object.values(currentData.parsedHL7Data.enhancedSegments).reduce((count, seg) => count + (seg.fields?.length || 0), 0) : 0;

            parsingResults.innerHTML = `
                <div class="transformation-summary">
                    ${this.getTransformationSummarySection(messageType, segments, fields, atomicMappings.length)}
                    ${this.getHL7ElementTableSection(atomicMappings, resourceTypes)}
                </div>
            `;

            this.showNotification('HL7 message transformed to FHIR resources!', 'success');
        }
    }

    /**
     * Toggle showing all fields vs only fields with values
     */
    toggleFieldsWithValues() {
        const btn = document.getElementById('fieldsToggleBtn');
        const allRowsWithoutValues = document.querySelectorAll('.field-without-value');

        if (btn.textContent.includes('Show All Fields')) {
            // Currently showing only fields with values, switch to show all
            allRowsWithoutValues.forEach(row => {
                row.style.display = '';
            });
            btn.innerHTML = '👁️ Show Only With Values';
            this.showNotification('Showing all fields including empty ones', 'info');
        } else {
            // Currently showing all fields, switch to show only with values
            allRowsWithoutValues.forEach(row => {
                row.style.display = 'none';
            });
            btn.innerHTML = '👁️ Show All Fields';
            this.showNotification('Showing only fields with values', 'info');
        }
    }

    /**
     * Get segment icon (schema-driven with fallback)
     */
    getSegmentIcon(segmentName) {
        // Try to get from schema first, then fallback to static icons
        const segmentDefinition = this.getSegmentDefinition(segmentName);
        if (segmentDefinition?.icon) {
            return segmentDefinition.icon;
        }

        // Fallback to static icons
        const icons = {
            'MSH': '📨', 'PID': '👤', 'PV1': '🏥', 'OBX': '🔬', 'ORC': '📝',
            'EVN': '⚡', 'NK1': '👥', 'AL1': '⚠️', 'DG1': '🩺', 'PR1': '💉',
            'GT1': '💳', 'IN1': '🛡️', 'NTE': '📄', 'OBR': '📋'
        };
        return icons[segmentName] || '📄';
    }

    /**
     * Get segment description (schema-driven with fallback)
     */
    getSegmentDescription(segmentName) {
        // Try to get from schema first
        const segmentDefinition = this.getSegmentDefinition(segmentName);
        if (segmentDefinition?.description) {
            return segmentDefinition.description;
        }

        // Fallback to static descriptions
        const descriptions = {
            'MSH': 'Message Header',
            'PID': 'Patient Identification',
            'PV1': 'Patient Visit',
            'OBX': 'Observation/Result',
            'ORC': 'Common Order',
            'EVN': 'Event Type',
            'NK1': 'Next of Kin/Associated Parties',
            'AL1': 'Patient Allergy Information',
            'DG1': 'Diagnosis',
            'PR1': 'Procedures',
            'GT1': 'Guarantor',
            'IN1': 'Insurance',
            'NTE': 'Notes and Comments',
            'OBR': 'Observation Request'
        };
        return descriptions[segmentName] || 'Unknown Segment';
    }

    /**
     * Get segment definition from schema/dictionary service
     */
    getSegmentDefinition(segmentName) {
        try {
            // First try to get from current parsed data (might contain schema info)
            const currentData = window.wizardController?.model?.getCurrentStepData();
            const parsedData = currentData?.parsedHL7Data;

            if (parsedData?.schemaDefinitions?.[segmentName]) {
                return parsedData.schemaDefinitions[segmentName];
            }

            // Try to get from enhanced segments data (may have definitions)
            if (parsedData?.enhancedSegments?.[segmentName]?.definition) {
                return parsedData.enhancedSegments[segmentName].definition;
            }

            // Cache for schema definitions
            if (!this.schemaCache) {
                this.schemaCache = new Map();
            }

            // Check cache first
            if (this.schemaCache.has(segmentName)) {
                return this.schemaCache.get(segmentName);
            }

            // Make API call to dictionary service (async, but we'll cache for next time)
            this.fetchSegmentDefinition(segmentName);

            return null; // Return null for now, definition will be cached for future use

        } catch (error) {
            console.warn('Error getting segment definition:', error);
            return null;
        }
    }

    /**
     * Fetch segment definition from dictionary service (async)
     */
    async fetchSegmentDefinition(segmentName) {
        // Skip API calls since the endpoint doesn't exist yet
        // Return a basic fallback definition
        console.log(`📋 Using fallback definition for segment ${segmentName}`);

        const fallbackDefinition = {
            segmentName: segmentName,
            description: `${segmentName} segment`,
            fields: [],
            version: '2.5'
        };

        if (!this.schemaCache) {
            this.schemaCache = new Map();
        }

        this.schemaCache.set(segmentName, fallbackDefinition);
        return fallbackDefinition;
    }

    /**
     * Refresh segment display with updated definition (optional enhancement)
     */
    refreshSegmentDisplay(segmentName, definition) {
        // Find segment cards that might need updating
        const segmentCards = document.querySelectorAll(`[data-segment="${segmentName}"]`);
        segmentCards.forEach(card => {
            const descriptionElement = card.querySelector('.segment-description');
            if (descriptionElement && definition.description) {
                descriptionElement.textContent = definition.description;
            }
        });
    }

    /**
     * Get field definition from schema/dictionary service
     */
    getFieldDefinition(segmentName, fieldPosition) {
        try {
            // First try to get from segment definition
            const segmentDefinition = this.getSegmentDefinition(segmentName);
            if (segmentDefinition?.fields) {
                // Convert position to index (position is 1-based, array is 0-based)
                const fieldIndex = parseInt(fieldPosition) - 1;
                if (fieldIndex >= 0 && fieldIndex < segmentDefinition.fields.length) {
                    return segmentDefinition.fields[fieldIndex];
                }
            }

            // Try to get from current parsed data
            const currentData = window.wizardController?.model?.getCurrentStepData();
            const parsedData = currentData?.parsedHL7Data;

            if (parsedData?.fieldDefinitions?.[segmentName]?.[fieldPosition]) {
                return parsedData.fieldDefinitions[segmentName][fieldPosition];
            }

            return null;

        } catch (error) {
            console.warn(`Error getting field definition for ${segmentName}.${fieldPosition}:`, error);
            return null;
        }
    }

    // ====================================
    // SEGMENT VIEWER HELPER METHODS (FROM BACKUP)
    // ====================================

    /**
     * Build field metadata cache from actual API data
     */
    buildFieldMetadataCache(segments) {
        console.log('🔍 Building field metadata cache from API data...');
        if (!this.fieldMetadataCache) {
            this.fieldMetadataCache = new Map();
        }
        this.fieldMetadataCache.clear();

        segments.forEach(([segmentName, segment]) => {
            if (!segment.fields) return;

            segment.fields.forEach(field => {
                const fieldKey = field.key;
                this.fieldMetadataCache.set(fieldKey, {
                    name: field.name || `Field ${field.position}`,
                    description: field.description || `${segmentName} field ${field.position}`,
                    dataType: field.dataType || 'ST',
                    optionality: field.optionality || 'O',
                    cardinality: field.cardinality || '[0..1]',
                    length: field.length,
                    tableId: field.tableId,
                    position: field.position,
                    hasValue: field.hasValue,
                    value: field.value
                });

                // Cache subfield metadata
                if (field.subfields && field.subfields.length > 0) {
                    field.subfields.forEach(subfield => {
                        this.fieldMetadataCache.set(subfield.key, {
                            name: subfield.name || `Component ${subfield.position}`,
                            description: subfield.description || `Component ${subfield.position} of ${fieldKey}`,
                            dataType: subfield.dataType || 'ST',
                            usage: subfield.usage || 'O',
                            length: subfield.length,
                            tableId: subfield.tableId,
                            position: subfield.position,
                            hasValue: subfield.hasValue,
                            value: subfield.value
                        });
                    });
                }
            });
        });

        console.log(`✅ Cached metadata for ${this.fieldMetadataCache.size} fields/components`);
    }

    /**
     * Get dynamic schema information
     */
    getSchemaInfo(data) {
        let schemaUsed = false;
        let schemaSource = 'Basic Parser';

        if (data.schemaLoaded) {
            schemaUsed = true;
            schemaSource = 'HL7 Schema';
        } else if (data.dictionaryUsed) {
            schemaUsed = true;
            schemaSource = 'Dictionary';
        } else {
            // Check segments for schema usage
            Object.values(data.enhancedSegments || {}).forEach(segment => {
                if (segment.dictionarySource &&
                    (segment.dictionarySource.includes('Schema') ||
                     segment.dictionarySource.includes('RealSchemaLoader'))) {
                    schemaUsed = true;
                    schemaSource = 'HL7 Schema';
                }
            });
        }

        return {
            used: schemaUsed,
            source: schemaSource,
            html: schemaUsed ?
                `<span class="schema-indicator schema-loaded">📋 ${schemaSource}</span>` :
                `<span class="schema-indicator basic-parser">💡 ${schemaSource}</span>`
        };
    }

    /**
     * Render dynamic badges based on actual data, no hardcoding
     */
    renderDynamicBadges(segName, segment, segmentErrors) {
        const badges = [];

        // Error/warning badges based on actual errors
        if (segmentErrors.some(err => err.severity === 'ERROR' || err.severity === 'error')) {
            badges.push('<span class="badge-mini error" title="Has errors">!</span>');
        } else if (segmentErrors.some(err => err.severity === 'WARNING' || err.severity === 'warning')) {
            badges.push('<span class="badge-mini warning" title="Has warnings">⚠</span>');
        }

        // Required badge based on actual segment data
        if (segment.required === true) {
            badges.push('<span class="badge-mini required" title="Required segment">R</span>');
        }

        // Repeating badge based on actual segment data
        if (segment.repeating === true) {
            badges.push('<span class="badge-mini repeating" title="Repeating segment">*</span>');
        }

        // Custom segment badge (Z segments)
        if (segName.startsWith('Z')) {
            badges.push('<span class="badge-mini custom" title="Custom segment">Z</span>');
        }

        return badges.join('');
    }

    /**
     * Render required fields badge
     */
    renderRequiredFieldsBadge(missingRequiredFields) {
        if (missingRequiredFields.length === 0) return '';

        return `<span class="badge-mini missing-required" title="${missingRequiredFields.length} missing required field${missingRequiredFields.length > 1 ? 's' : ''}">${missingRequiredFields.length}!</span>`;
    }

    /**
     * Get dynamic key fields based on actual field data and values
     */
    getKeyFields(segName, segment) {
        if (!segment.fields || segment.fields.length === 0) {
            return [];
        }

        const keyFields = [];

        // Take first few fields that have values, instead of hardcoded positions
        const fieldsWithValues = segment.fields
            .filter(field => field.hasValue && field.value)
            .slice(0, 3); // Take first 3 fields with values

        fieldsWithValues.forEach(field => {
            keyFields.push(`${field.key}:${this.truncateValue(field.value, 15)}`);
        });

        // If no fields have values, show first few field keys
        if (keyFields.length === 0) {
            segment.fields.slice(0, 2).forEach(field => {
                keyFields.push(`${field.key}:—`);
            });
        }

        return keyFields;
    }

    /**
     * Group fields for horizontal display
     */
    groupFields(fields) {
        const groups = [];
        const groupSize = 2; // 2 fields per row for better readability

        for (let i = 0; i < fields.length; i += groupSize) {
            groups.push(fields.slice(i, i + groupSize));
        }

        return groups;
    }

    /**
     * Render compact field display
     */
    renderCompactField(segName, field, fieldErrors) {
        const fieldValidationErrors = fieldErrors.filter(err => err.field === field.key);
        const hasIssues = fieldValidationErrors.length > 0;
        const isExpanded = this.expandedFields.has(`${segName}.${field.key}`);

        // Check if this specific field is missing and required
        const isMissingRequired = fieldValidationErrors.some(err =>
            err.code === 'MISSING_REQUIRED_FIELD' && err.field === field.key
        );
        const isEmptyRequired = fieldValidationErrors.some(err =>
            err.code === 'EMPTY_REQUIRED_FIELD' && err.field === field.key
        );

        // Check if this is a required field
        const isRequired = field.optionality === 'R';

        return `
            <div class="field-compact ${hasIssues ? 'has-issues' : ''} ${isMissingRequired ? 'missing-required' : ''} ${isEmptyRequired ? 'empty-required' : ''} ${isRequired ? 'required-field' : ''}"
                 onclick="window.wizardView.toggleField('${segName}', '${field.key}')">
                <div class="field-header-compact">
                    <div class="field-label">
                        <span class="field-key">${field.key}</span>
                        ${isRequired ? '<span class="required-indicator">*</span>' : ''}
                        <span class="field-name">${this.truncateText(field.name || `Field ${field.position}`, 25)}</span>
                        ${isMissingRequired ? '<span class="missing-required-icon" title="Required field is missing">❌</span>' : ''}
                        ${isEmptyRequired ? '<span class="empty-required-icon" title="Required field is empty">⚠️</span>' : ''}
                        ${hasIssues && !isMissingRequired && !isEmptyRequired ? '<span class="field-error-icon">!</span>' : ''}
                    </div>
                    <div class="field-value-preview">
                        ${field.hasValue ?
                            `<span class="value-text">${this.truncateValue(field.value, 35)}</span>` :
                            `<span class="no-value ${isMissingRequired || isEmptyRequired ? 'missing-required-value' : ''}">—</span>`
                        }
                        ${field.dataType && field.dataType !== 'ST' ? `<span class="data-type">(${field.dataType})</span>` : ''}
                    </div>
                </div>

                ${isExpanded ? this.renderFieldDetails(segName, field, fieldValidationErrors) : ''}
            </div>
        `;
    }

    /**
     * Render field details when expanded
     */
    renderFieldDetails(segName, field, fieldValidationErrors) {
        return `
            <div class="field-details-compact">
                <div class="field-metadata-row">
                    <span>Type: ${field.dataType || 'ST'}</span>
                    <span>Usage: ${this.getUsageDescription(field.optionality || 'O')}</span>
                    ${field.cardinality ? `<span>Repeat: ${field.cardinality}</span>` : ''}
                    ${field.length ? `<span>Max Length: ${field.length}</span>` : ''}
                    <span>Position: ${field.position}</span>
                </div>

                ${field.description ? `
                    <div class="field-description-row">
                        <strong>Description:</strong> ${field.description}
                    </div>
                ` : ''}

                ${field.hasValue && field.value ? `
                    <div class="field-value-row">
                        <strong>Full Value:</strong>
                        <code class="value-display">${this.escapeHtml(field.value)}</code>
                        ${this.analyzeValue(field)}
                    </div>
                ` : ''}

                ${field.subfields && field.subfields.length > 0 ? `
                    <div class="subfields-section">
                        <strong>Components (${field.subfields.length}):</strong>
                        ${this.renderEnhancedSubfields(field.subfields)}
                    </div>
                ` : ''}

                ${fieldValidationErrors.length > 0 ? `
                    <div class="field-validation-row">
                        ${fieldValidationErrors.map(error => `
                            <div class="validation-item ${error.severity}">
                                ${error.message}
                                ${error.suggestion ? `<br><em>💡 ${error.suggestion}</em>` : ''}
                            </div>
                        `).join('')}
                    </div>
                ` : ''}
            </div>
        `;
    }

    /**
     * Render enhanced subfields with complete metadata
     */
    renderEnhancedSubfields(subfields) {
        if (!subfields || subfields.length === 0) {
            return '<div class="no-subfields">No components defined</div>';
        }

        // Sort subfields by position
        const sortedSubfields = [...subfields].sort((a, b) => {
            if (a.position !== b.position) {
                return a.position - b.position;
            }
            return (a.sequence || 0) - (b.sequence || 0);
        });

        return `
            <div class="rich-subfields-container">
                <div class="subfields-header">
                    <span class="subfields-title">Components (${sortedSubfields.length})</span>
                    <button class="subfields-expand-btn" onclick="event.stopPropagation(); this.parentElement.parentElement.classList.toggle('subfields-expanded')">
                        <span class="expand-icon">▼</span> Details
                    </button>
                </div>
                <div class="rich-subfields-list">
                    ${sortedSubfields.map((subfield, index) => this.renderEnhancedSubfield(subfield, index)).join('')}
                </div>
            </div>
        `;
    }

    /**
     * Render individual subfield with complete metadata
     */
    renderEnhancedSubfield(subfield, index) {
        const isRequired = subfield.usage === 'R';
        const isEmpty = !subfield.hasValue || !subfield.value;
        const isMissingRequired = isRequired && isEmpty;

        return `
            <div class="enhanced-subfield ${subfield.hasValue ? 'has-value' : 'no-value'} ${isMissingRequired ? 'missing-required' : ''}"
                 onclick="event.stopPropagation(); this.classList.toggle('subfield-expanded')">

                <!-- Compact subfield header -->
                <div class="subfield-header-compact">
                    <div class="subfield-label-section">
                        <span class="subfield-position">${subfield.position}</span>
                        <span class="subfield-name">${subfield.name || `Component ${subfield.position}`}</span>
                        ${isRequired ? '<span class="required-indicator">*</span>' : ''}
                        ${isMissingRequired ? '<span class="missing-required-icon">⚠️</span>' : ''}
                    </div>

                    <div class="subfield-value-section">
                        <span class="subfield-value ${isEmpty ? 'empty-value' : ''}">
                            ${subfield.hasValue ? this.escapeHtml(this.truncateValue(subfield.value, 20)) : '—'}
                        </span>
                        ${subfield.dataType && subfield.dataType !== 'ST' ?
                            `<span class="subfield-datatype">(${subfield.dataType})</span>` : ''
                        }
                    </div>

                    <span class="subfield-expand-indicator">▼</span>
                </div>

                <!-- Detailed subfield information (hidden by default) -->
                <div class="subfield-details-panel">
                    <div class="subfield-metadata-grid">
                        <div class="metadata-row">
                            <span class="metadata-label">Key:</span>
                            <span class="metadata-value">${subfield.key}</span>
                        </div>
                        <div class="metadata-row">
                            <span class="metadata-label">Data Type:</span>
                            <span class="metadata-value">${subfield.dataType || 'ST'}</span>
                        </div>
                        <div class="metadata-row">
                            <span class="metadata-label">Usage:</span>
                            <span class="metadata-value ${subfield.usage === 'R' ? 'required' : 'optional'}">
                                ${this.getUsageDescription(subfield.usage)}
                            </span>
                        </div>
                        ${subfield.length ? `
                            <div class="metadata-row">
                                <span class="metadata-label">Max Length:</span>
                                <span class="metadata-value">${subfield.length}</span>
                            </div>
                        ` : ''}
                        <div class="metadata-row">
                            <span class="metadata-label">Position:</span>
                            <span class="metadata-value">${subfield.position}</span>
                        </div>
                    </div>

                    ${subfield.description ? `
                        <div class="subfield-description-section">
                            <div class="description-label">Description:</div>
                            <div class="description-text">${subfield.description}</div>
                        </div>
                    ` : ''}

                    ${subfield.hasValue && subfield.value ? `
                        <div class="subfield-value-section-detailed">
                            <div class="value-label">Full Value:</div>
                            <div class="value-display">
                                <code>${this.escapeHtml(subfield.value)}</code>
                                <span class="value-stats">(${subfield.value.length} chars)</span>
                            </div>
                        </div>
                    ` : ''}
                </div>
            </div>
        `;
    }

    /**
     * Render table row for each segment
     */
    renderSegmentTableRow(segName, segment, validationErrors) {
        const segmentErrors = validationErrors.filter(err => err.segment === segName);
        const hasIssues = segmentErrors.length > 0;
        const keyFields = this.getKeyFields(segName, segment);

        return `
            <tr class="segment-table-row ${hasIssues ? 'has-issues' : ''}" onclick="window.wizardView.viewSegmentDetails('${segName}')">
                <td class="seg-name-cell">
                    <span class="seg-name">${segName}</span>
                    ${this.renderDynamicBadges(segName, segment, segmentErrors)}
                </td>
                <td class="seg-desc-cell">${this.truncateText(segment.description || `${segName} Segment`, 30)}</td>
                <td class="field-count-cell">${segment.fieldCount || segment.fields?.length || 0}</td>
                <td class="status-cell">
                    ${hasIssues ?
                        `<span class="status-badge error">${segmentErrors.length} issue${segmentErrors.length > 1 ? 's' : ''}</span>` :
                        `<span class="status-badge ok">✓</span>`
                    }
                </td>
                <td class="key-values-cell">${keyFields.join(', ')}</td>
                <td class="actions-cell">
                    <button class="action-btn" onclick="event.stopPropagation(); window.wizardView.viewSegmentDetails('${segName}')">
                        <span>👁️</span> View Details
                    </button>
                </td>
            </tr>
        `;
    }

    /**
     * Render inline validation with specific field highlighting
     */
    renderInlineValidation(segmentErrors, missingRequiredFields) {
        if (segmentErrors.length === 0) return '';

        // Prioritize showing missing required fields
        if (missingRequiredFields && missingRequiredFields.length > 0) {
            return this.renderInlineMissingRequired(missingRequiredFields);
        }

        // Otherwise show first error
        const firstError = segmentErrors[0];
        return `
            <div class="inline-validation">
                <span class="validation-icon">${firstError.severity === 'error' || firstError.severity === 'ERROR' ? '❌' : '⚠️'}</span>
                <span class="validation-text">${this.truncateText(firstError.message, 60)}</span>
                ${segmentErrors.length > 1 ? `<span class="more-count">+${segmentErrors.length - 1} more</span>` : ''}
            </div>
        `;
    }

    /**
     * Inline missing required fields display
     */
    renderInlineMissingRequired(missingRequiredFields) {
        const fieldNames = missingRequiredFields.slice(0, 3).map(err => {
            return this.getFieldDisplayName(err.field);
        });

        const displayText = fieldNames.join(', ');
        const remainingCount = missingRequiredFields.length - 3;

        return `
            <div class="inline-validation missing-required">
                <span class="validation-icon">❌</span>
                <span class="validation-text">Missing required: ${displayText}</span>
                ${remainingCount > 0 ? `<span class="more-count">+${remainingCount} more</span>` : ''}
            </div>
        `;
    }

    /**
     * Get field display name from cache or derive from field key
     */
    getFieldDisplayName(fieldKey) {
        const cached = this.fieldMetadataCache?.get(fieldKey);
        if (cached && cached.name) {
            return `${fieldKey} (${cached.name})`;
        }

        // Fallback: extract from field key
        const parts = fieldKey.split('.');
        if (parts.length === 2) {
            return `${fieldKey} (Field ${parts[1]})`;
        }

        return fieldKey;
    }

    /**
     * Get human-readable usage descriptions
     */
    getUsageDescription(usage) {
        const usageMap = {
            'R': 'Required',
            'O': 'Optional',
            'C': 'Conditional',
            'B': 'Backward Compatible',
            'X': 'Not Supported'
        };
        return usageMap[usage] || usage || 'Optional';
    }

    /**
     * Analyze field value to show helpful information
     */
    analyzeValue(field) {
        if (!field.hasValue || !field.value) return '';

        const value = field.value;
        const analysis = [];

        // Component analysis
        if (value.includes('^')) {
            const components = value.split('^');
            analysis.push(`${components.length} components`);
        }

        // Date parsing for timestamp fields
        if (field.dataType === 'TS' || field.dataType === 'DT') {
            const dateMatch = value.match(/(\d{8})/);
            if (dateMatch) {
                const dateStr = dateMatch[1];
                const formatted = `${dateStr.substr(4,2)}/${dateStr.substr(6,2)}/${dateStr.substr(0,4)}`;
                analysis.push(`Date: ${formatted}`);
            }
        }

        // Length analysis
        if (value.length > 20) {
            analysis.push(`${value.length} chars`);
        }

        if (analysis.length > 0) {
            return `<span class="value-analysis">(${analysis.join(', ')})</span>`;
        }

        return '';
    }

    /**
     * Helper methods
     */
    truncateText(text, maxLength) {
        if (!text) return '';
        return text.length > maxLength ? text.substring(0, maxLength) + '...' : text;
    }

    truncateValue(value, maxLength = 30) {
        if (!value) return '';
        const str = String(value);
        return str.length > maxLength ? str.substring(0, maxLength) + '...' : str;
    }

    escapeHtml(text) {
        if (!text) return '';
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    // ====================================
    // SEGMENT VIEWER INTERACTION METHODS
    // ====================================

    /**
     * Toggle segment expansion
     */
    toggleSegment(segName) {
        if (!this.expandedSegments) {
            this.expandedSegments = new Set(['MSH', 'PID']);
        }

        if (this.expandedSegments.has(segName)) {
            this.expandedSegments.delete(segName);
        } else {
            this.expandedSegments.add(segName);
        }

        this.refreshSegmentView();
    }

    /**
     * Toggle field expansion
     */
    toggleField(segName, fieldKey) {
        if (!this.expandedFields) {
            this.expandedFields = new Set();
        }

        const fieldId = `${segName}.${fieldKey}`;
        if (this.expandedFields.has(fieldId)) {
            this.expandedFields.delete(fieldId);
        } else {
            this.expandedFields.add(fieldId);
        }

        this.refreshSegmentView();
    }

    /**
     * Change view mode
     */
    setViewMode(mode) {
        console.log('🔄 Switching view mode to:', mode);
        this.viewMode = mode;
        this.refreshSegmentView();

        // Update button states
        setTimeout(() => {
            document.querySelectorAll('.view-btn').forEach(btn => {
                btn.classList.remove('active');
            });

            const activeBtn = document.querySelector(`.view-btn[onclick*="${mode}"]`);
            if (activeBtn) {
                activeBtn.classList.add('active');
            }
        }, 50);
    }

    /**
     * Toggle segments based on available data
     */
    toggleAllSegments() {
        console.log('🔍 Toggle all segments clicked');

        // Get segments that exist in the current data
        const existingSegments = [];
        const currentData = window.wizardController?.model?.getCurrentStepData();
        if (currentData?.parsedHL7Data?.enhancedSegments) {
            const availableSegments = Object.keys(currentData.parsedHL7Data.enhancedSegments);
            // Take first 3 segments as "important" dynamically
            existingSegments.push(...availableSegments.slice(0, 3));
        }

        if (!this.expandedSegments) {
            this.expandedSegments = new Set();
        }

        const hasExpanded = existingSegments.some(seg => this.expandedSegments.has(seg));

        if (hasExpanded) {
            // Collapse all important segments
            console.log('🔽 Collapsing important segments');
            existingSegments.forEach(seg => this.expandedSegments.delete(seg));
        } else {
            // Expand all important segments
            console.log('🔼 Expanding important segments:', existingSegments);
            existingSegments.forEach(seg => this.expandedSegments.add(seg));
        }

        // Switch to compact view if in table view
        if (this.viewMode === 'table') {
            this.viewMode = 'compact';
        }

        this.refreshSegmentView();
    }

    /**
     * View segment details - switches to compact view and shows the segment
     */
    viewSegmentDetails(segName) {
        console.log('🔍 View segment details:', segName);

        this.viewMode = 'compact';
        if (!this.expandedSegments) {
            this.expandedSegments = new Set();
        }
        this.expandedSegments.add(segName);

        this.refreshSegmentView();

        // Scroll to the segment after rendering
        setTimeout(() => {
            const segmentCards = document.querySelectorAll('.segment-compact');
            segmentCards.forEach(card => {
                const segmentRow = card.querySelector('.segment-row');
                if (segmentRow && segmentRow.onclick && segmentRow.onclick.toString().includes(segName)) {
                    card.scrollIntoView({ behavior: 'smooth', block: 'center' });
                    card.style.boxShadow = '0 4px 16px rgba(59, 130, 246, 0.3)';
                    setTimeout(() => {
                        card.style.boxShadow = '';
                    }, 2000);
                }
            });
        }, 200);
    }

    /**
     * Refresh the current segment view
     */
    refreshSegmentView() {
        console.log('🔄 Refreshing segment viewer, mode:', this.viewMode);

        // Debug: Check what containers are available
        const allContainers = document.querySelectorAll('[id*="parsed"], [id*="Data"], [id*="Review"], [class*="transformation"]');
        console.log('🔍 Available containers:', Array.from(allContainers).map(el => ({id: el.id, class: el.className})));

        const currentData = window.wizardController?.model?.getCurrentStepData();
        if (currentData?.parsedHL7Data) {
            // Try the correct container names
            let container = document.getElementById('parsingResults') || document.getElementById('parsedDataReview');

            // Fallback: Try to find any results container
            if (!container) {
                container = document.querySelector('[id*="result"], [id*="transformation"], [class*="results"], [class*="parsing-results"]');
                console.log('🔍 Using fallback container:', container?.id || 'none found');
            }

            if (container) {
                const newContent = this.getTransformationResultsTemplate(currentData);
                container.innerHTML = newContent;
                console.log('✅ Segment view refreshed successfully');
            } else {
                console.warn('⚠️ Container not found for refresh - available elements:', document.querySelectorAll('*[id]').length);
            }
        } else {
            console.warn('⚠️ No parsed data available for refresh');
        }
    }


}

// Global function for expand/collapse functionality
window.toggleFieldExpansion = function(fieldId) {
    if (window.wizardView && typeof window.wizardView.toggleFieldExpansion === 'function') {
        window.wizardView.toggleFieldExpansion(fieldId);
    } else {
        console.warn('WizardView instance not available');
    }
};

// Global function for segment toggle
window.toggleSegment = function(segName) {
    if (window.wizardView && typeof window.wizardView.toggleSegment === 'function') {
        window.wizardView.toggleSegment(segName);
    } else {
        console.warn('WizardView instance not available for segment toggle');
    }
};

// Global function for field visibility toggle
window.toggleFieldVisibility = function(element, segName) {
    if (window.wizardView && typeof window.wizardView.toggleFieldVisibility === 'function') {
        window.wizardView.toggleFieldVisibility(element, segName);
    } else {
        console.warn('WizardView instance not available for field visibility toggle');
    }
};

// Global function for HL7 table display
window.showHL7Table = function(tableId, fieldName, element) {
    console.log(`📋 Showing HL7 Table ${tableId} for field: ${fieldName}`);

    // Get HL7 table data
    const tableData = getHL7TableData(tableId);

    if (!tableData || tableData.length === 0) {
        showTableNotFound(tableId, fieldName);
        return;
    }

    // Create and show table modal
    showTableModal(tableId, fieldName, tableData, element);
};

// Check if HL7 table has data
window.hasHL7TableData = function(tableId) {
    const tableData = getHL7TableData(tableId);
    return tableData && tableData.length > 0;
};

// HL7 table data lookup function
function getHL7TableData(tableId) {
    // Common HL7 tables - this would ideally come from the schema/API
    const hl7Tables = {
        '0001': [
            {code: 'M', description: 'Male'},
            {code: 'F', description: 'Female'},
            {code: 'O', description: 'Other'},
            {code: 'U', description: 'Unknown'}
        ],
        '0002': [
            {code: 'A', description: 'Married'},
            {code: 'B', description: 'Unmarried'},
            {code: 'C', description: 'Common law'},
            {code: 'D', description: 'Divorced'},
            {code: 'E', description: 'Legally separated'},
            {code: 'I', description: 'Interlocutory'},
            {code: 'M', description: 'Married'},
            {code: 'P', description: 'Polygamous'},
            {code: 'S', description: 'Never married'},
            {code: 'T', description: 'Unreported'},
            {code: 'U', description: 'Unknown'},
            {code: 'W', description: 'Widowed'}
        ],
        '0003': [
            {code: 'A01', description: 'ADT/ACK - Admit/visit notification'},
            {code: 'A02', description: 'ADT/ACK - Transfer a patient'},
            {code: 'A03', description: 'ADT/ACK - Discharge/end visit'},
            {code: 'A04', description: 'ADT/ACK - Register a patient'},
            {code: 'A05', description: 'ADT/ACK - Pre-admit a patient'},
            {code: 'A08', description: 'ADT/ACK - Update patient information'},
            {code: 'A11', description: 'ADT/ACK - Cancel admit/visit notification'},
            {code: 'A12', description: 'ADT/ACK - Cancel transfer'},
            {code: 'A13', description: 'ADT/ACK - Cancel discharge/end visit'}
        ],
        '0004': [
            {code: 'E', description: 'Emergency'},
            {code: 'I', description: 'Inpatient'},
            {code: 'O', description: 'Outpatient'},
            {code: 'P', description: 'Preadmit'},
            {code: 'R', description: 'Recurring patient'},
            {code: 'B', description: 'Obstetrics'},
            {code: 'C', description: 'Commercial Account'},
            {code: 'N', description: 'Not Applicable'},
            {code: 'U', description: 'Unknown'}
        ],
        '0063': [
            {code: 'A', description: 'Analogue'},
            {code: 'D', description: 'Digital'},
            {code: 'FAX', description: 'Facsimile'},
            {code: 'MODEM', description: 'Modem'},
            {code: 'PHONE', description: 'Telephone'},
            {code: 'TTY', description: 'Teletypewriter'}
        ],
        '0091': [
            {code: '1', description: 'Emergency contact'},
            {code: '2', description: 'Federal agency'},
            {code: '3', description: 'Insurance company'},
            {code: '4', description: 'Next-of-kin'},
            {code: '5', description: 'State agency'},
            {code: 'C', description: 'Emergency contact'},
            {code: 'E', description: 'Employer'},
            {code: 'F', description: 'Federal agency'},
            {code: 'I', description: 'Insurance company'},
            {code: 'N', description: 'Next-of-kin'},
            {code: 'S', description: 'State agency'},
            {code: 'U', description: 'Unknown'}
        ]
    };

    return hl7Tables[tableId] || null;
}

// Show table not found message
function showTableNotFound(tableId, fieldName) {
    const modal = createTableModal();
    modal.innerHTML = `
        <div class="table-modal-content">
            <div class="table-modal-header">
                <h3>HL7 Table T${tableId}</h3>
                <button class="table-modal-close" onclick="closeTableModal()">&times;</button>
            </div>
            <div class="table-modal-body">
                <div style="text-align: center; padding: 40px; color: #6b7280;">
                    <div style="font-size: 48px; margin-bottom: 16px;">📋</div>
                    <h4>Table T${tableId} Not Available</h4>
                    <p>Field: <strong>${fieldName}</strong></p>
                    <p style="font-size: 14px; color: #9ca3af; margin-top: 12px;">
                        This table definition is not currently loaded in the system.
                    </p>
                </div>
            </div>
        </div>
    `;
    document.body.appendChild(modal);
}

// Show table modal with data
function showTableModal(tableId, fieldName, tableData, triggerElement) {
    const modal = createTableModal();
    modal.innerHTML = `
        <div class="table-modal-content">
            <div class="table-modal-header">
                <h3>HL7 Table T${tableId}</h3>
                <button class="table-modal-close" onclick="closeTableModal()">&times;</button>
            </div>
            <div class="table-modal-body">
                <div style="margin-bottom: 16px; padding: 12px; background: #f8fafc; border-radius: 6px; border-left: 4px solid #ec4899;">
                    <strong>Field:</strong> ${fieldName}
                </div>
                <div class="table-values-container">
                    <table class="hl7-table-values">
                        <thead>
                            <tr>
                                <th style="background: #1e3a8a; color: white; padding: 8px; text-align: left;">Code</th>
                                <th style="background: #1e3a8a; color: white; padding: 8px; text-align: left;">Description</th>
                            </tr>
                        </thead>
                        <tbody>
                            ${tableData.map(item => `
                                <tr style="border-bottom: 1px solid #e5e7eb;">
                                    <td style="padding: 8px; font-family: monospace; font-weight: 600; color: #1e3a8a;">${item.code}</td>
                                    <td style="padding: 8px; color: #374151;">${item.description}</td>
                                </tr>
                            `).join('')}
                        </tbody>
                    </table>
                </div>
                <div style="margin-top: 16px; text-align: center; font-size: 12px; color: #6b7280;">
                    ${tableData.length} possible values
                </div>
            </div>
        </div>
    `;
    document.body.appendChild(modal);
}

// Create table modal element
function createTableModal() {
    const modal = document.createElement('div');
    modal.className = 'table-modal-overlay';
    modal.style.cssText = `
        position: fixed;
        top: 0;
        left: 0;
        width: 100%;
        height: 100%;
        background: rgba(0, 0, 0, 0.5);
        z-index: 10000;
        display: flex;
        align-items: center;
        justify-content: center;
        backdrop-filter: blur(2px);
    `;

    modal.onclick = function(e) {
        if (e.target === modal) {
            closeTableModal();
        }
    };

    return modal;
}

// Close table modal
window.closeTableModal = function() {
    const modal = document.querySelector('.table-modal-overlay');
    if (modal) {
        modal.remove();
    }
};

// Add CSS for table modal
const tableModalCSS = `
    .table-modal-content {
        background: white;
        border-radius: 12px;
        box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.1), 0 10px 10px -5px rgba(0, 0, 0, 0.04);
        max-width: 600px;
        width: 90%;
        max-height: 80vh;
        overflow: hidden;
        animation: modalSlideIn 0.3s ease-out;
    }

    .table-modal-header {
        background: linear-gradient(135deg, #1e3a8a 0%, #1e40af 100%);
        color: white;
        padding: 16px 20px;
        display: flex;
        justify-content: space-between;
        align-items: center;
    }

    .table-modal-header h3 {
        margin: 0;
        font-size: 18px;
        font-weight: 600;
    }

    .table-modal-close {
        background: none;
        border: none;
        color: white;
        font-size: 24px;
        cursor: pointer;
        padding: 0;
        width: 30px;
        height: 30px;
        display: flex;
        align-items: center;
        justify-content: center;
        border-radius: 50%;
        transition: background 0.2s;
    }

    .table-modal-close:hover {
        background: rgba(255, 255, 255, 0.2);
    }

    .table-modal-body {
        padding: 20px;
        max-height: 60vh;
        overflow-y: auto;
    }

    .hl7-table-values {
        width: 100%;
        border-collapse: collapse;
        border-radius: 6px;
        overflow: hidden;
        box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
    }

    .hl7-table-values th {
        font-size: 14px;
        font-weight: 600;
        text-transform: uppercase;
        letter-spacing: 0.5px;
    }

    .hl7-table-values td {
        font-size: 14px;
    }

    .hl7-table-values tbody tr:hover {
        background: #f8fafc;
    }

    @keyframes modalSlideIn {
        from {
            opacity: 0;
            transform: translateY(-20px) scale(0.95);
        }
        to {
            opacity: 1;
            transform: translateY(0) scale(1);
        }
    }

    @keyframes loading-slide {
        0% {
            transform: translateX(-100%);
        }
        100% {
            transform: translateX(100%);
        }
    }
`;

// Add CSS to document
if (!document.getElementById('table-modal-styles')) {
    const style = document.createElement('style');
    style.id = 'table-modal-styles';
    style.textContent = tableModalCSS;
    document.head.appendChild(style);
}

// ── AI Suggest Mappings ──────────────────────────────────────────────────────
window.aiSuggestMappings = async function() {
    console.log('🤖 aiSuggestMappings called, AIAssistant:', window.AIAssistant);

    // Get the HL7 sample from the wizard model (step 2) — fall back to OOB sample if skipped
    const wzCtrl = window.wizardController;
    const userHL7  = wzCtrl?.model?.data?.hl7Message || '';
    // Resolve the active message type from the highlighted pill, then the model
    const activePillBtn = document.querySelector('.msg-type-preview-btn[style*="background: rgb(30, 58, 138)"]')
        || document.querySelector('.msg-type-preview-btn.active');
    const msgType = wzCtrl?.model?.data?.detectedMessageType
        || activePillBtn?.dataset?.msgType
        || wzCtrl?.model?.data?.messageType
        || 'ADT^A01';

    let hl7Sample = userHL7;
    if (!hl7Sample) {
        // User skipped step 2 — use the built-in OOB sample for the active message type
        hl7Sample = window._SAMPLE_HL7?.[msgType] || window._SAMPLE_HL7?.['ADT^A01'] || '';
        if (!hl7Sample) {
            AppDialogs.toast('No HL7 sample available. Please complete Step 2 first.', 'warning');
            return;
        }
    }

    // Show the suggestion panel (inject if not present)
    let panel = document.getElementById('ai-mapping-suggestion-panel');
    if (!panel) {
        panel = document.createElement('div');
        panel.id = 'ai-mapping-suggestion-panel';
        panel.style.cssText = 'position:fixed;top:0;right:0;width:480px;height:100vh;background:#fff;border-left:1px solid #e2e8f0;box-shadow:-4px 0 24px rgba(30,58,138,0.12);z-index:10010;display:flex;flex-direction:column;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;';
        panel.innerHTML = `
<div style="background:linear-gradient(135deg,#f472b6,#1e3a8a);padding:14px 18px;display:flex;align-items:center;justify-content:space-between;color:#fff;flex-shrink:0;">
  <div>
    <div style="font-weight:700;font-size:15px;">💡 AI Mapping Suggestions</div>
    <div style="font-size:11px;opacity:0.85;margin-top:2px;" id="ai-ms-subtitle">Analysing ${msgType || 'HL7'} → FHIR R4${!userHL7 ? ' (using OOB sample)' : ''}…</div>
  </div>
  <button onclick="document.getElementById('ai-mapping-suggestion-panel').remove()" style="background:rgba(255,255,255,0.15);border:none;color:#fff;font-size:18px;cursor:pointer;padding:4px 8px;border-radius:6px;">✕</button>
</div>
<div id="ai-ms-status" style="padding:10px 16px;font-size:12px;color:#6b7280;border-bottom:1px solid #f0f0f0;flex-shrink:0;">Fetching AI suggestions…</div>
<div id="ai-ms-table" style="flex:1;overflow-y:auto;padding:12px 16px;"></div>
<div id="ai-ms-footer" style="padding:12px 16px;border-top:1px solid #e2e8f0;flex-shrink:0;display:none;">
  <button id="ai-ms-apply-btn" style="width:100%;background:linear-gradient(135deg,#f472b6,#1e3a8a);color:#fff;border:none;padding:10px;border-radius:8px;font-size:13px;font-weight:600;cursor:pointer;">Apply accepted mappings</button>
</div>`;
        document.body.appendChild(panel);
    } else {
        panel.style.display = 'flex';
    }

    const statusEl = document.getElementById('ai-ms-status');
    const tableEl  = document.getElementById('ai-ms-table');
    const footerEl = document.getElementById('ai-ms-footer');
    const applyBtn = document.getElementById('ai-ms-apply-btn');

    statusEl.textContent = 'Analysing HL7 message — this may take up to 2 minutes on CPU…';
    tableEl.innerHTML    = '<div style="text-align:center;padding:40px;color:#94a3b8;line-height:1.8;">🤖 AI is generating mapping suggestions…<br><span style="font-size:11px;color:#cbd5e1;">Running on CPU — typically 30–90 seconds</span></div>';

    // Helper: call the suggest-mappings API directly (works whether AIAssistant widget is loaded or not)
    async function callSuggestMappingsAPI(message, targetFormat) {
        if (window.AIAssistant) {
            return window.AIAssistant.suggestMappings(message, targetFormat);
        }
        const controller = new AbortController();
        const timer = setTimeout(() => controller.abort(), 200000); // 200s client timeout
        try {
            const resp = await fetch('/api/ai/suggest-mappings', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                credentials: 'include',
                signal: controller.signal,
                body: JSON.stringify({ message, target_format: targetFormat })
            });
            clearTimeout(timer);
            const text = await resp.text();
            try { return JSON.parse(text); } catch {
                throw new Error(resp.status === 404
                    ? 'AI service endpoint not found — check that the Go backend has started.'
                    : `AI service returned HTTP ${resp.status}. Make sure the Go backend is running.`);
            }
        } catch (e) {
            clearTimeout(timer);
            if (e.name === 'AbortError') throw new Error('AI request timed out after 200s — the model may be overloaded.');
            throw e;
        }
    }

    // ── Collect already-mapped HL7 source fields from the wizard model ──────────
    function getExistingMappedSources() {
        const existing = new Set();
        try {
            const data = window.wizardController?.getCurrentData?.() || {};
            const atoms = data?.fhirTransformResult?.atomicMappings || data?.atomicMappings || [];
            atoms.forEach(m => {
                const src = m.sourcePath || m.hl7Path || m.hl7Field || '';
                if (src) existing.add(src.trim());
            });
        } catch (_) {}
        return existing;
    }

    // ── Transform key resolution ─────────────────────────────────────────────
    // Prefer the suggestion's engine-native transform_key. Fall back to inferring
    // from data_type only when transform_key is absent (e.g. LLM-generated suggestions).
    function resolveTransformKey(suggestion) {
        if (suggestion.transform_key) return suggestion.transform_key;
        // Generic inference from data_type (legacy / LLM path)
        if (suggestion.data_type === 'TS' || suggestion.data_type === 'date') return 'ts_to_date';
        if (suggestion.data_type === 'IS' || suggestion.data_type === 'code') return 'gender_mapping';
        return '';
    }
    // Wizard dropdown value (direct/lookup/format/custom) derived from engine key
    function wizardTransformLabel(transformKey) {
        if (!transformKey) return 'direct';
        if (transformKey.includes('to_date') || transformKey.includes('to_datetime')) return 'format';
        if (transformKey.includes('gender') || transformKey.includes('lookup') ||
            transformKey.includes('status') || transformKey.includes('flag') ||
            transformKey.includes('class')) return 'lookup';
        if (transformKey.includes('to_humanname') || transformKey.includes('to_address') ||
            transformKey.includes('to_identifier') || transformKey.includes('to_contactpoint') ||
            transformKey.includes('to_codeableconcept')) return 'custom';
        return 'direct';
    }

    // ── Determine FHIR resource type from target path ────────────────────────
    function resourceTypeFromFHIR(fhirPath) {
        const m = fhirPath.match(/^([A-Z][a-zA-Z]+)\./);
        return m ? m[1] : 'Unknown';
    }

    let suggestions = [];
    try {
        const result = await callSuggestMappingsAPI(hl7Sample, 'fhir_r4');
        const allSuggestions = result?.data?.suggestions || [];

        if (allSuggestions.length === 0) {
            tableEl.innerHTML = '<div style="text-align:center;padding:40px;color:#94a3b8;">No suggestions returned. Try a longer HL7 sample.</div>';
            statusEl.textContent = 'No suggestions found.';
            return;
        }

        // Filter out already-mapped fields
        const mapped = getExistingMappedSources();
        suggestions = allSuggestions.filter(s => !mapped.has(s.source_field));
        const alreadyMapped = allSuggestions.filter(s => mapped.has(s.source_field));

        // Gap/status summary header
        const gapCount = suggestions.length;
        const okCount  = alreadyMapped.length;
        const totalFields = allSuggestions.length;
        const coverage = totalFields > 0 ? Math.round((okCount / totalFields) * 100) : 0;
        const analysisLabel = userHL7 ? `fields found in your HL7 message` : `OOB template fields`;
        const summaryHtml = `
<div style="background:#f0f9ff;border:1px solid #bae6fd;border-radius:8px;padding:10px 12px;margin-bottom:10px;font-size:12px;">
  <div style="font-weight:700;color:#0369a1;margin-bottom:6px;">📊 ${totalFields} ${analysisLabel} — ${coverage}% covered (${okCount} mapped, ${gapCount} gap${gapCount !== 1 ? 's' : ''})</div>
  ${okCount > 0 ? `<div style="color:#16a34a;margin-bottom:3px;">✅ Already mapped (${okCount}): ${alreadyMapped.map(s=>`<span style="font-family:monospace;background:#dcfce7;padding:1px 4px;border-radius:3px;margin:1px;">${s.source_field}</span>`).join(' ')}</div>` : ''}
  ${gapCount > 0 ? `<div style="color:#d97706;">⚠️ Gaps found (${gapCount}): ${suggestions.map(s=>`<span style="font-family:monospace;background:#fef9c3;padding:1px 4px;border-radius:3px;margin:1px;">${s.source_field}</span>`).join(' ')}</div>` : `<div style="color:#16a34a;">✅ All ${analysisLabel} are mapped.</div>`}
</div>`;

        if (suggestions.length === 0) {
            tableEl.innerHTML = summaryHtml + `<div style="text-align:center;padding:24px;color:#16a34a;font-weight:600;">✅ No gaps — all ${analysisLabel} are already mapped!</div>`;
            statusEl.textContent = `Coverage: ${coverage}% — nothing missing.`;
            footerEl.style.display = 'none';
            return;
        }

        statusEl.textContent = `${gapCount} gap(s) found — accept mappings to add them:`;

        // Render gap rows
        tableEl.innerHTML = summaryHtml + suggestions.map((s, i) => {
            const conf    = Math.round((s.confidence > 0 ? s.confidence : 0.7) * 100);
            const confCol = conf >= 80 ? '#16a34a' : conf >= 50 ? '#d97706' : '#dc2626';
            const txKey   = resolveTransformKey(s);
            const txType  = wizardTransformLabel(txKey);
            const txLabel = txKey
                ? `<span title="${txKey}">${txType === 'format' ? '📅' : txType === 'lookup' ? '🔄' : txType === 'custom' ? '⚙️' : '⇒'} ${txKey}</span>`
                : '⇒ Direct';
            return `
<div class="ai-ms-row" data-idx="${i}" style="border:1px solid #e2e8f0;border-radius:8px;padding:12px;margin-bottom:8px;background:#fafafa;">
  <div style="display:flex;justify-content:space-between;align-items:flex-start;gap:8px;">
    <div style="flex:1;min-width:0;">
      <div style="font-size:12px;font-weight:600;color:#1e3a8a;margin-bottom:3px;">
        <span style="font-family:monospace;">${s.source_field}</span>
        <span style="color:#6b7280;margin:0 4px;">→</span>
        <span style="font-family:monospace;">${s.target_field}</span>
      </div>
      <div style="font-size:11px;color:#6b7280;line-height:1.4;">${s.reasoning || ''}</div>
      <div style="margin-top:4px;"><span style="font-size:10px;background:#f1f5f9;color:#475569;padding:1px 6px;border-radius:9px;">${txLabel}</span></div>
    </div>
    <div style="display:flex;flex-direction:column;align-items:flex-end;gap:6px;flex-shrink:0;">
      <span style="font-size:11px;font-weight:600;color:${confCol};">${conf}% conf.</span>
      <div style="display:flex;gap:5px;">
        <button class="ai-ms-accept" data-idx="${i}" style="background:#dcfce7;color:#166534;border:1px solid #bbf7d0;padding:3px 10px;border-radius:12px;cursor:pointer;font-size:11px;font-weight:600;">✓ Accept</button>
        <button class="ai-ms-reject" data-idx="${i}" style="background:#fef2f2;color:#991b1b;border:1px solid #fecaca;padding:3px 10px;border-radius:12px;cursor:pointer;font-size:11px;font-weight:600;">✕ Reject</button>
      </div>
    </div>
  </div>
</div>`;
        }).join('');

        footerEl.style.display = '';

        // Wire accept/reject buttons
        tableEl.querySelectorAll('.ai-ms-accept').forEach(btn => {
            btn.addEventListener('click', () => {
                const row = tableEl.querySelector(`.ai-ms-row[data-idx="${btn.dataset.idx}"]`);
                row.style.background = '#f0fdf4';
                row.style.borderColor = '#86efac';
                btn.textContent = '✓ Accepted';
                btn.style.background = '#16a34a';
                btn.style.color = '#fff';
                btn.disabled = true;
                row.querySelector('.ai-ms-reject').disabled = true;
                suggestions[+btn.dataset.idx]._accepted = true;
            });
        });
        tableEl.querySelectorAll('.ai-ms-reject').forEach(btn => {
            btn.addEventListener('click', () => {
                const row = tableEl.querySelector(`.ai-ms-row[data-idx="${btn.dataset.idx}"]`);
                row.style.opacity = '0.4';
                row.style.background = '#fef2f2';
                btn.textContent = '✕ Rejected';
                btn.disabled = true;
                row.querySelector('.ai-ms-accept').disabled = true;
                suggestions[+btn.dataset.idx]._rejected = true;
                if (window.AIAssistant && wzCtrl?.model?.data?.interfaceId) {
                    window.AIAssistant.feedbackMapping(
                        wzCtrl.model.data.interfaceId,
                        suggestions[+btn.dataset.idx].source_field,
                        suggestions[+btn.dataset.idx].target_field,
                        false, 'rejected by user in wizard'
                    );
                }
            });
        });

        // Apply accepted → actually insert into atomicMappings and re-render the wizard step
        applyBtn.addEventListener('click', () => {
            const accepted = suggestions.filter(s => s._accepted);
            if (accepted.length === 0) { AppDialogs.toast('Accept at least one suggestion first.', 'warning'); return; }

            const data = window.wizardController?.getCurrentData?.();
            if (!data) { AppDialogs.toast('Wizard model not available.', 'error'); return; }

            if (!data.fhirTransformResult) data.fhirTransformResult = {};
            if (!data.fhirTransformResult.atomicMappings) data.fhirTransformResult.atomicMappings = [];

            const atoms = data.fhirTransformResult.atomicMappings;
            let added = 0;
            accepted.forEach(s => {
                const alreadyIn = atoms.some(m =>
                    (m.sourcePath || m.hl7Path || m.hl7Field) === s.source_field);
                if (!alreadyIn) {
                    const txKey   = resolveTransformKey(s);
                    const txLabel = wizardTransformLabel(txKey);
                    atoms.push({
                        sourcePath:       s.source_field,
                        hl7Path:          s.source_field,
                        targetPath:       s.target_field,
                        fhirPath:         s.target_field,
                        resourceType:     resourceTypeFromFHIR(s.target_field),
                        // DataTypeTransform is what the Go engine dispatches on.
                        // transform/transformType drive the wizard dropdown display.
                        DataTypeTransform: txKey,
                        transform:         txLabel,
                        transformType:     txLabel,
                        confidence:        s.confidence || 0.9,
                        dataType:          s.data_type || '',
                        isRequired:        false,
                        source:            'ai_suggest',
                    });
                    added++;
                    // Feedback to KB
                    if (window.AIAssistant && wzCtrl?.model?.data?.interfaceId) {
                        window.AIAssistant.feedbackMapping(
                            wzCtrl.model.data.interfaceId,
                            s.source_field, s.target_field, true, s.reasoning || ''
                        );
                    }
                }
            });

            // Re-render wizard step to show new mappings
            if (added > 0 && window.wizardView) {
                const step = window.wizardController?.model?.currentStep;
                window.wizardView.renderStep(step, data);
                AppDialogs.toast(`✅ ${added} mapping(s) added to wizard.`, 'success');
            }

            applyBtn.textContent = `✓ ${added} mapping(s) added`;
            applyBtn.style.background = '#16a34a';
            setTimeout(() => {
                panel.remove();
            }, 2000);
        });

    } catch (err) {
        const isServiceDown = err.message.includes('not found') || err.message.includes('unexpected response') || err.message.includes('Failed to fetch');
        tableEl.innerHTML = `
            <div style="text-align:center;padding:40px;">
                <div style="font-size:32px;margin-bottom:12px;">⚠️</div>
                <div style="color:#dc2626;font-weight:600;margin-bottom:8px;">AI suggestion failed</div>
                <div style="color:#6b7280;font-size:12px;line-height:1.5;">
                    ${isServiceDown
                        ? 'The ezCompanion AI service is not running.<br>Start the Go backend with Ollama configured to use AI suggestions.'
                        : err.message.replace(/</g, '&lt;')}
                </div>
            </div>`;
        statusEl.textContent = isServiceDown ? 'AI service unavailable.' : 'Error fetching suggestions.';
    }
};

// Keep old name as alias so any surviving references don't 404
window.saveFHIRMappingConfiguration = function() { window.applyAndContinue(); };

window.applyAndContinue = function() {
    const ctrl = window.wizardController;
    if (!ctrl) { showNotification('Wizard not initialised', 'error'); return; }

    const btn = document.getElementById('btn-apply-mappings');
    if (btn) { btn.disabled = true; btn.textContent = '⏳ Saving…'; }

    // Collect any manual edits from visible mapping rows
    const customMappings = [];
    document.querySelectorAll('.fhir-mapping-row').forEach(row => {
        const hl7Field = row.querySelector('.hl7-source-select')?.value
                      || row.querySelector('input[type="text"]')?.value;
        const fhirField = row.querySelector('.fhir-target-select')?.value
                       || row.querySelectorAll('input[type="text"]')[1]?.value;
        const transformation = row.querySelector('.transformation-select')?.value || 'direct';
        if (hl7Field && fhirField) {
            customMappings.push({
                hl7Path: hl7Field, fhirPath: fhirField,
                transformation, enabled: !row.classList.contains('disabled')
            });
        }
    });

    // Merge into model (prefer transform-result mappings when no manual edits were made)
    const existingMappings = ctrl.model.data.fhirTransformResult?.atomicMappings || [];
    ctrl.model.data.customMappings = customMappings.length > 0 ? customMappings : existingMappings;
    ctrl.model.data.mappingOptions  = {
        ...(ctrl.model.data.mappingOptions || {}),
        validationLevel:       document.getElementById('validation-level')?.value       || 'moderate',
        transformationPreset:  document.getElementById('transformation-preset')?.value  || 'standard',
        missingFieldHandling:  document.getElementById('missing-field-handling')?.value || 'skip',
        appliedAt: new Date().toISOString()
    };

    // Advance wizard — click the real Next button so all validation hooks run
    const nextBtn = document.getElementById('wizardNext');
    if (nextBtn && !nextBtn.disabled) {
        nextBtn.click();
    } else {
        showNotification('Complete required fields before continuing', 'warning');
        if (btn) { btn.disabled = false; btn.innerHTML = '✓ Apply &amp; Continue'; }
    }
};

window.testFHIRMappings = async function() {
    console.log('🧪 Testing FHIR transformation...');

    const data = window.wizardController?.getCurrentData();

    if (!data?.parsedHL7Data) {
        showNotification('No HL7 data available to test. Please parse HL7 first.', 'warning');
        return;
    }

    if (!data?.fhirTransformResult?.atomicMappings || data.fhirTransformResult.atomicMappings.length === 0) {
        showNotification('No mappings available to test.', 'warning');
        return;
    }

    // Show testing indicator
    const testButton = document.getElementById('btn-test-mappings') || document.querySelector('button[onclick="window.testFHIRMappings()"]');
    const originalText = testButton?.innerHTML || '🧪 Test Transform';
    if (testButton) {
        testButton.innerHTML = '🔄 Testing...';
        testButton.disabled = true;
    }

    try {
        // Re-run the transformation to test current mappings
        console.log('🔄 Re-running transformation...');

        const response = await fetch('http://localhost:8080/api/fhir/test-transform-v3', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                parsedHL7Data: data.parsedHL7Data
            })
        });

        if (!response.ok) {
            throw new Error(`Transformation failed: ${response.status}`);
        }

        const result = await response.json();

        // Analyze results
        const totalMappings = result.atomicMappings?.length || 0;
        const successfulMappings = result.atomicMappings?.filter(m => m.transformType !== 'error').length || 0;
        const resourcesGenerated = result.fhirResources?.length || 0;
        const warnings = result.warnings?.length || 0;

        // Build result message
        let message = `🧪 Transformation Test Results:\n\n`;
        message += `✅ Mappings Applied: ${successfulMappings}/${totalMappings}\n`;
        message += `📦 FHIR Resources Generated: ${resourcesGenerated}\n`;

        if (warnings > 0) {
            message += `⚠️ Warnings: ${warnings}\n`;
        }

        console.log('✅ Test completed:', result);

        // Show results
        if (warnings === 0) {
            showNotification(`✅ Test passed! ${successfulMappings} mappings applied, ${resourcesGenerated} resources generated.`, 'success');
        } else {
            showNotification(`⚠️ Test completed with ${warnings} warnings. ${successfulMappings} mappings applied.`, 'warning');
            console.warn('Transformation warnings:', result.warnings);
        }

    } catch (error) {
        console.error('❌ Test failed:', error);
        showNotification(`Test failed: ${error.message}`, 'error');
    } finally {
        // Reset button
        if (testButton) {
            testButton.innerHTML = originalText;
            testButton.disabled = false;
        }
    }
};

window.updateMapping = function(rowId, field, value) {
    console.log(`🔄 Updating mapping ${rowId}: ${field} = ${value}`);

    const row = document.querySelector(`[data-mapping-id="${rowId}"]`);
    if (!row) {
        console.error('❌ Mapping row not found:', rowId);
        return;
    }

    // Update the specific field
    const fieldElement = row.querySelector(`.${field}-select, .${field}-input`);
    if (fieldElement) {
        fieldElement.value = value;

        // Add visual feedback
        fieldElement.style.background = '#dbeafe';
        setTimeout(() => {
            fieldElement.style.background = '';
        }, 1000);

        // Update confidence if this is a critical field change
        if (field === 'hl7-source' || field === 'fhir-target') {
            const confidenceBadge = row.querySelector('.confidence-badge');
            if (confidenceBadge) {
                const newConfidence = value ? '0.90' : '0.50';
                confidenceBadge.textContent = newConfidence;
                confidenceBadge.style.background = value ? '#16a34a' : '#f59e0b';
            }
        }

        console.log('✅ Mapping updated successfully');
    } else {
        console.error('❌ Field element not found:', field);
    }
};

window.toggleMappingView = function(viewType) {
    const groupsContainer = document.getElementById('resource-mapping-groups');
    if (!groupsContainer) return;

    const cards    = groupsContainer.querySelectorAll('.resource-group-card');
    const headers  = groupsContainer.querySelectorAll('.resource-group-header');
    const rowLists = groupsContainer.querySelectorAll('[id^="resource-mappings-"]');
    const allRows  = groupsContainer.querySelectorAll('.fhir-mapping-row');

    switch (viewType) {
        case 'grouped':
        default:
            // Resource cards with headers — default card layout
            cards.forEach(c => { c.style.display = 'block'; c.style.marginBottom = ''; });
            headers.forEach(h => { h.style.display = ''; });
            rowLists.forEach(l => { l.style.maxHeight = '400px'; l.style.borderTop = ''; });
            allRows.forEach(r => { r.style.display = ''; r.style.borderRadius = ''; r.style.borderLeft = ''; });
            groupsContainer.style.gap = '16px';
            break;

        case 'list':
            // Flat dense table — hide resource headers, remove card gaps, show all rows
            cards.forEach(c => { c.style.marginBottom = '0'; c.style.borderRadius = '0'; c.style.border = 'none'; c.style.boxShadow = 'none'; });
            headers.forEach(h => { h.style.display = 'none'; });
            rowLists.forEach(l => { l.style.maxHeight = 'none'; l.style.borderTop = '1px solid #e5e7eb'; });
            allRows.forEach(r => { r.style.display = ''; r.style.borderRadius = '0'; r.style.borderLeft = '4px solid #e0e7ff'; });
            groupsContainer.style.gap = '0';
            // Wrap in a single bordered container on first call
            if (!groupsContainer.style.border) {
                groupsContainer.style.border = '2px solid #e2e8f0';
                groupsContainer.style.borderRadius = '8px';
                groupsContainer.style.overflow = 'hidden';
            }
            break;

        case 'validation': {
            // Show only rows with low confidence or validation issues, hide the rest
            cards.forEach(c => { c.style.display = 'block'; c.style.marginBottom = ''; });
            headers.forEach(h => { h.style.display = ''; });
            rowLists.forEach(l => { l.style.maxHeight = 'none'; });
            groupsContainer.style.gap = '16px';

            let issueCount = 0;
            const data = window.wizardController?.getCurrentData();
            const mappings = data?.fhirTransformResult?.atomicMappings || [];

            allRows.forEach((row, i) => {
                const m = mappings[i];
                const hasIssue = (m?.validationIssues?.length > 0) || (m?.confidence != null && m.confidence < 0.75);
                row.style.display = hasIssue ? '' : 'none';
                if (hasIssue) issueCount++;
            });

            // Hide cards where every row is hidden
            cards.forEach(card => {
                const visible = [...card.querySelectorAll('.fhir-mapping-row')].some(r => r.style.display !== 'none');
                card.style.display = visible ? 'block' : 'none';
            });

            document.getElementById('no-issues-msg')?.remove();
            break;
        }
    }

    // Restore to grouped removes the flat-list wrapper styles
    if (viewType === 'grouped') {
        groupsContainer.style.border = '';
        groupsContainer.style.borderRadius = '';
        groupsContainer.style.overflow = '';
        cards.forEach(c => { c.style.border = ''; c.style.borderRadius = '12px'; c.style.boxShadow = ''; });
        document.getElementById('no-issues-msg')?.remove();
    }
};

window.filterMappingsByResource = function(resourceType) {
    console.log('🔍 Filtering mappings by resource:', resourceType);

    const mappingRows = document.querySelectorAll('.fhir-mapping-row');
    let visibleCount = 0;

    mappingRows.forEach(row => {
        const fhirPath = row.querySelector('.fhir-target-select')?.value || '';
        const shouldShow = resourceType === 'all' || fhirPath.startsWith(resourceType + '.');

        row.style.display = shouldShow ? 'flex' : 'none';
        if (shouldShow) visibleCount++;
    });

    // Update filter button states
    document.querySelectorAll('.resource-filter-btn').forEach(btn => {
        btn.classList.remove('active');
        if (btn.dataset.resource === resourceType) {
            btn.classList.add('active');
        }
    });

    console.log(`✅ Filtered to ${visibleCount} mappings for resource: ${resourceType}`);
};

window.exportFHIRConfiguration = function() {
    console.log('📤 Exporting FHIR configuration...');

    const wizardData = window.wizardController?.getCurrentData() || {};
    const exportData = {
        interface: wizardData.interfaceConfig || {},
        mappings: wizardData.fhirMappings || [],
        configuration: wizardData.mappingConfiguration || {},
        hl7Schema: wizardData.hl7Data || {},
        fhirResources: wizardData.fhirResult?.fhirResources || [],
        exportedAt: new Date().toISOString(),
        version: '1.0'
    };

    // Create download
    const blob = new Blob([JSON.stringify(exportData, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);

    const a = document.createElement('a');
    a.href = url;
    a.download = `fhir-mapping-config-${Date.now()}.json`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);

    console.log('✅ Configuration exported successfully');
    showNotification('Configuration exported successfully!', 'success');
};

// Helper function for notifications
function showNotification(message, type = 'info') {
    const notification = document.createElement('div');
    notification.style.cssText = `
        position: fixed;
        top: 20px;
        right: 20px;
        padding: 12px 20px;
        border-radius: 8px;
        color: white;
        font-weight: 500;
        z-index: 10000;
        animation: slideInRight 0.3s ease-out;
    `;

    switch (type) {
        case 'success':
            notification.style.background = '#16a34a';
            break;
        case 'error':
            notification.style.background = '#dc2626';
            break;
        case 'warning':
            notification.style.background = '#f59e0b';
            break;
        default:
            notification.style.background = '#3b82f6';
    }

    notification.textContent = message;
    document.body.appendChild(notification);

    setTimeout(() => {
        notification.remove();
    }, 4000);
}

// Add animation CSS for notifications
if (!document.getElementById('notification-styles')) {
    const style = document.createElement('style');
    style.id = 'notification-styles';
    style.textContent = `
        @keyframes slideInRight {
            from {
                transform: translateX(100%);
                opacity: 0;
            }
            to {
                transform: translateX(0);
                opacity: 1;
            }
        }
    `;
    document.head.appendChild(style);
}

// Make available globally
// Add missing Step 4 helper functions for comprehensive mapping interface
window.toggleResourceMappings = function(resourceType) {
    const content = document.getElementById(`resource-content-${resourceType}`);
    const icon = document.getElementById(`toggle-icon-${resourceType}`);

    if (content && icon) {
        if (content.style.display === 'none') {
            content.style.display = 'block';
            icon.textContent = '▼';
        } else {
            content.style.display = 'none';
            icon.textContent = '▶';
        }
    }
};

window.editFHIRMapping = function(hl7Field, fhirPath, value) {
    console.log('🔧 Edit FHIR mapping:', { hl7Field, fhirPath, value });

    // Get available HL7 fields from parsed data AND from existing mappings (to include subfields)
    const data = window.wizardController?.model?.data;
    const hl7FieldsSet = new Set();

    // Add base fields from parsed data
    if (data?.parsedHL7Data?.basicSegments) {
        Object.keys(data.parsedHL7Data.basicSegments).forEach(segment => {
            const fields = data.parsedHL7Data.basicSegments[segment].fields;
            Object.keys(fields).forEach(fieldKey => {
                hl7FieldsSet.add(fieldKey);
            });
        });
    }

    // Also add HL7 paths from existing mappings (includes subfields like PID.3.1)
    if (data?.fhirTransformResult?.atomicMappings) {
        data.fhirTransformResult.atomicMappings.forEach(m => {
            const source = m.sourcePath || m.hl7Path || m.hl7Field || '';
            if (source) {
                hl7FieldsSet.add(source);
            }
        });
    }

    const hl7Fields = Array.from(hl7FieldsSet).sort();
    console.log('📋 Available HL7 fields:', hl7Fields.length, 'Current field:', hl7Field);

    // Get available FHIR paths from existing mappings
    const fhirPaths = [];
    if (data?.fhirTransformResult?.atomicMappings) {
        data.fhirTransformResult.atomicMappings.forEach(m => {
            const path = m.targetPath || m.fhirPath || '';
            if (path && !fhirPaths.includes(path)) {
                fhirPaths.push(path);
            }
        });
    }

    console.log('📋 Available FHIR paths:', fhirPaths.length, 'Current path:', fhirPath);

    // Create enhanced edit modal with dropdown + free text
    const modal = document.createElement('div');
    modal.id = 'edit-mapping-modal';
    modal.style.cssText = 'position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.5); z-index: 10000; display: flex; align-items: center; justify-content: center;';

    modal.innerHTML = `
        <div style="background: white; border-radius: 12px; padding: 24px; max-width: 700px; width: 90%; max-height: 90vh; overflow-y: auto;">
            <h3 style="margin: 0 0 8px 0; color: #1e3a8a;">✏️ Edit Field Mapping</h3>
            <p style="margin: 0 0 20px 0; color: #6b7280; font-size: 14px;">Select from dropdown or enter custom path. Paths will be validated.</p>

            <!-- HL7 Field Section -->
            <div style="margin-bottom: 20px;">
                <label style="display: block; margin-bottom: 8px; font-weight: 600; color: #374151;">📥 HL7 Source Field:</label>
                <select id="edit-hl7-dropdown" style="width: 100%; padding: 10px; border: 1px solid #e2e8f0; border-radius: 6px; margin-bottom: 8px;" onchange="document.getElementById('edit-hl7-field').value = this.value; window.validateHL7Path();">
                    <option value="">-- Select HL7 Field --</option>
                    ${hl7Fields.map(f => `<option value="${f}" ${f === hl7Field ? 'selected' : ''}>${f}</option>`).join('')}
                </select>
                <input type="text" id="edit-hl7-field" value="${hl7Field}" placeholder="Or enter custom path (e.g., PID.5.1)" style="width: 100%; padding: 10px; border: 1px solid #e2e8f0; border-radius: 6px;" oninput="window.validateHL7Path()">
                <div id="hl7-validation" style="margin-top: 4px; font-size: 12px; min-height: 16px;"></div>
            </div>

            <!-- FHIR Path Section -->
            <div style="margin-bottom: 20px;">
                <label style="display: block; margin-bottom: 8px; font-weight: 600; color: #374151;">📤 FHIR Target Path:</label>
                <select id="edit-fhir-dropdown" style="width: 100%; padding: 10px; border: 1px solid #e2e8f0; border-radius: 6px; margin-bottom: 8px;" onchange="document.getElementById('edit-fhir-path').value = this.value; window.validateFHIRPath();">
                    <option value="">-- Select FHIR Path --</option>
                    ${fhirPaths.map(f => `<option value="${f}" ${f === fhirPath ? 'selected' : ''}>${f}</option>`).join('')}
                </select>
                <input type="text" id="edit-fhir-path" value="${fhirPath}" placeholder="Or enter custom path (e.g., Patient.name[0].family)" style="width: 100%; padding: 10px; border: 1px solid #e2e8f0; border-radius: 6px;" oninput="window.validateFHIRPath()">
                <div id="fhir-validation" style="margin-top: 4px; font-size: 12px; min-height: 16px;"></div>
            </div>

            <!-- Value Preview Section -->
            <div style="margin-bottom: 20px; padding: 12px; background: #f9fafb; border-radius: 6px; border: 1px solid #e5e7eb;">
                <label style="display: block; margin-bottom: 8px; font-weight: 600; color: #374151;">🔍 Current Values:</label>
                <div id="value-preview" style="font-size: 13px; color: #6b7280;">${value || 'No value available'}</div>
                <div style="margin-top: 8px; font-size: 12px; color: #9ca3af;">
                    <em>This shows the current HL7 → FHIR value transformation. The "Value" field is read-only and displays the mapping result.</em>
                </div>
            </div>

            <!-- Action Buttons -->
            <div style="display: flex; gap: 12px; justify-content: flex-end;">
                <button onclick="window.closeMappingEditModal()" style="padding: 10px 20px; background: #f3f4f6; color: #374151; border: none; border-radius: 6px; cursor: pointer; font-weight: 500;">Cancel</button>
                <button id="save-mapping-btn" onclick="window.saveMappingEdit()" style="padding: 10px 20px; background: #1e3a8a; color: white; border: none; border-radius: 6px; cursor: pointer; font-weight: 500;">💾 Save Changes</button>
            </div>
        </div>
    `;

    document.body.appendChild(modal);

    // Initial validation
    window.validateHL7Path();
    window.validateFHIRPath();
};

window.saveMappingEdit = function() {
    console.log('💾 Attempting to save mapping edit...');

    // Validate paths before saving
    const hl7Valid = window.validateHL7Path();
    const fhirValid = window.validateFHIRPath();

    if (!hl7Valid || !fhirValid) {
        console.warn('⚠️ Validation failed, cannot save mapping');
        return;
    }

    const hl7Field = document.getElementById('edit-hl7-field')?.value;
    const fhirPath = document.getElementById('edit-fhir-path')?.value;

    console.log('✅ Validation passed, saving mapping:', { hl7Field, fhirPath });

    // Update wizard data if available
    if (window.wizardController) {
        const data = window.wizardController.getCurrentData();
        if (data.fhirTransformResult && data.fhirTransformResult.atomicMappings) {
            // Find and update the mapping - use backend field names (sourcePath/targetPath)
            const mapping = data.fhirTransformResult.atomicMappings.find(m => {
                const mHl7 = m.sourcePath || m.hl7Path || m.hl7Field || '';
                const mFhir = m.targetPath || m.fhirPath || m.fhirField || '';
                return mHl7 === hl7Field || mFhir === fhirPath;
            });

            if (mapping) {
                // Update using correct field names
                mapping.sourcePath = hl7Field;
                mapping.targetPath = fhirPath;

                console.log('✅ Mapping updated:', mapping);

                // Close modal
                window.closeMappingEditModal();

                // Refresh the display with proper data
                if (window.wizardController) {
                    const currentStep = window.wizardController.model.currentStep;
                    const stepData = window.wizardController.getCurrentData();
                    console.log('🔄 Refreshing step with data:', stepData);
                    window.wizardController.view.renderStep(currentStep, stepData);
                }

                // Show success notification
                showNotification('Mapping updated successfully!', 'success');
            } else {
                console.warn('⚠️ Could not find mapping to update');
                showNotification('Could not find mapping to update', 'error');
            }
        }
    }
};

window.addResourceMapping = function(resourceType) {
    console.log('➕ Adding new mapping for resource:', resourceType);

    // Simple implementation - would be expanded in real use
    const modal = document.createElement('div');
    modal.style.cssText = 'position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.5); z-index: 10000; display: flex; align-items: center; justify-content: center;';

    modal.innerHTML = `
        <div style="background: white; border-radius: 12px; padding: 24px; max-width: 500px; width: 90%;">
            <h3 style="margin: 0 0 16px 0; color: #1e3a8a;">Add ${resourceType} Mapping</h3>
            <p style="color: #6b7280; margin-bottom: 20px;">This feature would allow adding custom field mappings for the ${resourceType} resource.</p>
            <button onclick="this.closest('[style*=\"position: fixed\"]').remove()" style="padding: 8px 16px; background: #1e3a8a; color: white; border: none; border-radius: 6px; cursor: pointer;">Close</button>
        </div>
    `;

    document.body.appendChild(modal);
};

window.toggleResourceGroup = function(resourceType) {
    console.log('👁️ Toggling resource group:', resourceType);

    const container = document.getElementById(`resource-mappings-${resourceType}`);
    if (container) {
        container.style.display = container.style.display === 'none' ? 'block' : 'none';
    }
};

window.updateMappingTransformation = function(index, transformType) {
    console.log('🔄 Updating mapping transformation:', index, transformType);

    // Update the transformation type for the mapping
    if (window.wizardController) {
        const data = window.wizardController.getCurrentData();
        if (data.fhirTransformResult && data.fhirTransformResult.atomicMappings) {
            if (data.fhirTransformResult.atomicMappings[index]) {
                data.fhirTransformResult.atomicMappings[index].transform = transformType;
            }
        }
    }
};

// Validation functions for HL7 and FHIR paths
window.validateHL7Path = function() {
    const input = document.getElementById('edit-hl7-field');
    const validationDiv = document.getElementById('hl7-validation');
    const value = input?.value || '';

    if (!value) {
        validationDiv.innerHTML = '<span style="color: #f59e0b;">⚠️ HL7 field is required</span>';
        return false;
    }

    // HL7 path pattern: SEGMENT.FIELD or SEGMENT.FIELD.SUBFIELD (e.g., PID.5 or PID.5.1)
    const hl7Pattern = /^[A-Z]{2,3}\.\d+(\.\d+)?$/;
    if (!hl7Pattern.test(value)) {
        validationDiv.innerHTML = '<span style="color: #dc2626;">❌ Invalid format. Expected: SEGMENT.FIELD or SEGMENT.FIELD.SUBFIELD (e.g., PID.5.1)</span>';
        return false;
    }

    validationDiv.innerHTML = '<span style="color: #16a34a;">✅ Valid HL7 path</span>';
    return true;
};

window.validateFHIRPath = function() {
    const input = document.getElementById('edit-fhir-path');
    const validationDiv = document.getElementById('fhir-validation');
    const value = input?.value || '';

    if (!value) {
        validationDiv.innerHTML = '<span style="color: #f59e0b;">⚠️ FHIR path is required</span>';
        return false;
    }

    // FHIR path pattern: Resource.field or Resource.field[0].subfield (e.g., Patient.name[0].family)
    const fhirPattern = /^[A-Z][a-zA-Z]+(\.[a-zA-Z]+(\[\d+\])?)+$/;
    if (!fhirPattern.test(value)) {
        validationDiv.innerHTML = '<span style="color: #dc2626;">❌ Invalid format. Expected: Resource.field or Resource.field[0].subfield (e.g., Patient.name[0].family)</span>';
        return false;
    }

    validationDiv.innerHTML = '<span style="color: #16a34a;">✅ Valid FHIR path</span>';
    return true;
};

window.closeMappingEditModal = function() {
    const modal = document.getElementById('edit-mapping-modal');
    if (modal && modal.parentNode) {
        modal.parentNode.removeChild(modal);
    }
};

window.editMappingDetails = function(index) {
    console.log('📝 Editing mapping details for index:', index);

    if (window.wizardController) {
        const data = window.wizardController.getCurrentData();
        if (data.fhirTransformResult && data.fhirTransformResult.atomicMappings) {
            const mapping = data.fhirTransformResult.atomicMappings[index];
            if (mapping) {
                // Use correct field names from backend: sourcePath and targetPath
                const hl7Field = mapping.sourcePath || mapping.hl7Path || mapping.hl7Field || '';
                const fhirPath = mapping.targetPath || mapping.fhirPath || mapping.fhirField || '';

                // Extract actual values using the view's helper functions
                const hl7Value = window.wizardController.view.extractHL7Value(hl7Field);
                const fhirValue = window.wizardController.view.extractFHIRValue(fhirPath, mapping.resourceType);

                // Pass both values as a descriptive string
                const valueDisplay = `HL7: "${hl7Value}" → FHIR: "${fhirValue}"`;

                window.editFHIRMapping(hl7Field, fhirPath, valueDisplay);
            }
        }
    }
};

window.deleteMappingRow = async function(index) {
    console.log('🗑️ Deleting mapping row:', index);

    const ok = await AppDialogs.confirm('Are you sure you want to delete this mapping?', { type: 'danger', title: 'Delete Mapping', confirmText: 'Delete' });
    if (ok) {
        if (window.wizardController) {
            const data = window.wizardController.getCurrentData();
            if (data.fhirTransformResult && data.fhirTransformResult.atomicMappings) {
                data.fhirTransformResult.atomicMappings.splice(index, 1);

                // Refresh the display
                if (window.wizardView) {
                    window.wizardView.renderStep(4);
                }
            }
        }
    }
};

window.generateFHIRMappings = async function() {
    console.log('🎯 Generating FHIR mappings...');

    // Check if transformation has already been done
    if (window.wizardController) {
        const data = window.wizardController.getCurrentData();

        // If mappings already exist, just re-render to show them
        if (data.fhirTransformResult && data.fhirTransformResult.atomicMappings) {
            console.log('📊 Mappings already exist, re-rendering view');
            const currentStep = window.wizardController.model.currentStep;
            window.wizardController.view.renderStep(currentStep, data);
            return;
        }

        if (data.parsedHL7Data) {
            console.log('📊 HL7 data available for transformation');

            // Show loading state
            const button = document.querySelector('button[onclick="window.generateFHIRMappings()"]');
            if (button) {
                button.textContent = '⏳ Generating...';
                button.disabled = true;
            }

            try {
                // Use the same transformation service as auto-transform
                const transformSvc = window.optimizedFHIRTransform || window.wizardFHIRTransform;
                if (transformSvc) {
                    console.log('🔄 Using existing transformation service');
                    await transformSvc.startOptimizedFHIRTransformation();
                    console.log('✅ Transformation completed via service');
                } else {
                    console.error('❌ Transformation service not available');
                    AppDialogs.toast('Transformation service not initialized. Please refresh the page.', 'error');
                }

                if (button) {
                    button.textContent = '✅ Mappings Generated';
                    button.disabled = false;
                }
            } catch (error) {
                console.error('❌ Error generating FHIR mappings:', error);
                AppDialogs.toast(`Failed to generate FHIR mappings: ${error.message}`, 'error');

                if (button) {
                    button.textContent = '🎯 Auto-Generate Mappings';
                    button.disabled = false;
                }
            }
        } else {
            console.warn('⚠️ No HL7 data available for transformation');
            AppDialogs.toast('Please complete Step 2 (HL7 Parsing) first before generating mappings.', 'warning');
        }
    }
};

window.createCustomMapping = function() {
    console.log('✨ Creating custom mapping...');

    const modal = document.createElement('div');
    modal.style.cssText = 'position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.5); z-index: 10000; display: flex; align-items: center; justify-content: center;';

    modal.innerHTML = `
        <div style="background: white; border-radius: 12px; padding: 24px; max-width: 600px; width: 90%;">
            <h3 style="margin: 0 0 16px 0; color: #1e3a8a;">Create Custom Mapping</h3>
            <p style="color: #6b7280; margin-bottom: 20px;">Define a custom HL7 to FHIR field mapping.</p>
            <div style="margin-bottom: 16px;">
                <label style="display: block; margin-bottom: 8px; font-weight: 600;">HL7 Field Path:</label>
                <input type="text" placeholder="e.g., PID.5.1" style="width: 100%; padding: 8px; border: 1px solid #e2e8f0; border-radius: 6px;">
            </div>
            <div style="margin-bottom: 16px;">
                <label style="display: block; margin-bottom: 8px; font-weight: 600;">FHIR Resource Path:</label>
                <input type="text" placeholder="e.g., Patient.name[0].family" style="width: 100%; padding: 8px; border: 1px solid #e2e8f0; border-radius: 6px;">
            </div>
            <div style="display: flex; gap: 12px; justify-content: flex-end;">
                <button onclick="this.closest('[style*=\"position: fixed\"]').remove()" style="padding: 8px 16px; background: #f3f4f6; border: none; border-radius: 6px; cursor: pointer;">Cancel</button>
                <button onclick="this.closest('[style*=\"position: fixed\"]').remove()" style="padding: 8px 16px; background: #1e3a8a; color: white; border: none; border-radius: 6px; cursor: pointer;">Create Mapping</button>
            </div>
        </div>
    `;

    document.body.appendChild(modal);
};

// Mapping interaction functions (Removed duplicates - using implementations above)

window.updateMapping = function(index, field, value) {
    console.log(`📝 Updating mapping ${index}, field ${field} to: ${value}`);
};

window.addResourceMapping = function(resourceType) {
    console.log(`➕ Adding mapping for resource: ${resourceType}`);
    window.createCustomMapping();
};

window.toggleResourceGroup = function(resourceType) {
    console.log(`👁️ Toggling resource group: ${resourceType}`);
    const container = document.getElementById(`resource-mappings-${resourceType}`);
    if (container) {
        container.style.display = container.style.display === 'none' ? 'block' : 'none';
    }
};

window.exportFHIRConfiguration = function() {
    console.log('📤 Exporting FHIR configuration...');
    const data = window.wizardController?.getCurrentData();
    if (data?.fhirTransformResult) {
        const config = {
            messageType: data.detectedMessageType,
            mappings: data.fhirTransformResult.atomicMappings,
            resources: data.fhirTransformResult.fhirResources,
            timestamp: new Date().toISOString()
        };
        const blob = new Blob([JSON.stringify(config, null, 2)], { type: 'application/json' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `fhir-mapping-${data.detectedMessageType}-${Date.now()}.json`;
        a.click();
        URL.revokeObjectURL(url);
    }
};

// Renders the transform indicator between HL7 source and FHIR target.
//   Direct  → gray "Direct" pill  (no transform, copy as-is)
//   Anything else → colored pill with human-readable label + tooltip of raw key
window._transformBadge = function(transformType, index) {
    const isDirect = !transformType || transformType === 'direct';
    if (isDirect) {
        return `<span style="display:inline-block;padding:2px 8px;border-radius:10px;font-size:10px;font-weight:500;background:#f3f4f6;color:#9ca3af;border:1px solid #e5e7eb;white-space:nowrap;" title="No transform — value copied as-is">Direct</span>`;
    }
    const label = window._transformLabel(transformType);
    return `<span style="display:inline-block;padding:2px 8px;border-radius:10px;font-size:10px;font-weight:600;background:#eff6ff;color:#2563eb;border:1px solid #bfdbfe;white-space:nowrap;cursor:help;max-width:110px;overflow:hidden;text-overflow:ellipsis;" title="Transform applied: ${transformType}">${label}</span>`;
};

// Returns the short human-readable label for a transform key.
window._transformLabel = function(key) {
    const MAP = {
        'ce_to_codeableconcept':           'CE → Code',
        'cwe_to_codeableconcept':          'CWE → Code',
        'cx_to_identifier':                'CX → ID',
        'xpn_to_humanname':                'XPN → Name',
        'xad_to_address':                  'XAD → Address',
        'xtn_to_contactpoint':             'XTN → Contact',
        'phone_to_contactpoint':           'Phone → Contact',
        'ts_to_datetime':                  'TS → DateTime',
        'ts_to_date':                      'TS → Date',
        'gender_mapping':                  'Gender',
        'administrative_sex':              'Gender',
        'msh9_trigger_event_to_coding':    'Event Coding',
        'obr_status_to_dr_status':         'DR Status',
        'obx_status_to_obs_status':        'Obs Status',
        'abnormal_flag_to_interpretation': 'Abnormal Flag',
        'obx_value_by_type':               'OBX Value',
        'static_value':                    'Static',
        'hl7_active_flag':                 'Y/N → Bool',
    };
    return MAP[key] || key;
};

// Full catalog of transform functions supported by the Go engine.
// Returns an HTML <option> string with the current value pre-selected.
// Unknown values (custom DB entries) get a passthrough "Custom: <value>" option.
window._hl7TransformOptions = function(current) {
    const TRANSFORMS = [
        // ── Copy ──────────────────────────────────────────────────────────────
        { value: 'direct',                         label: 'Direct (copy)' },
        // ── Coded / composite types ───────────────────────────────────────────
        { value: 'ce_to_codeableconcept',          label: 'CE → CodeableConcept' },
        { value: 'cwe_to_codeableconcept',         label: 'CWE → CodeableConcept' },
        { value: 'cx_to_identifier',               label: 'CX → Identifier' },
        { value: 'xpn_to_humanname',               label: 'XPN → HumanName' },
        { value: 'xad_to_address',                 label: 'XAD → Address' },
        { value: 'xtn_to_contactpoint',            label: 'XTN → ContactPoint' },
        { value: 'phone_to_contactpoint',          label: 'Phone → ContactPoint' },
        // ── Date / time ───────────────────────────────────────────────────────
        { value: 'ts_to_datetime',                 label: 'TS → dateTime' },
        { value: 'ts_to_date',                     label: 'TS → date' },
        // ── Patient / admin ───────────────────────────────────────────────────
        { value: 'gender_mapping',                 label: 'Gender (F/M/O → FHIR)' },
        { value: 'administrative_sex',             label: 'Administrative sex' },
        // ── Message header ────────────────────────────────────────────────────
        { value: 'msh9_trigger_event_to_coding',   label: 'MSH-9 → event Coding' },
        // ── ORU / lab ─────────────────────────────────────────────────────────
        { value: 'obr_status_to_dr_status',        label: 'OBR status → DR status' },
        { value: 'obx_status_to_obs_status',       label: 'OBX status → Obs status' },
        { value: 'abnormal_flag_to_interpretation',label: 'Abnormal flag → Interpretation' },
        { value: 'obx_value_by_type',              label: 'OBX value by OBX-2 type' },
    ];

    const known = TRANSFORMS.find(t => t.value === current);
    let options = TRANSFORMS.map(t =>
        `<option value="${t.value}"${t.value === current ? ' selected' : ''}>${t.label}</option>`
    ).join('');

    // If the current value isn't in the catalog, add it as a custom entry
    if (!known && current && current !== 'direct') {
        options = `<option value="${current}" selected>Custom: ${current}</option>` + options;
    }

    return options;
};

window.viewRawFHIRJSON = function() {
    console.log('📄 Viewing raw FHIR JSON...');
    const data = window.wizardController?.getCurrentData();

    if (!data?.fhirTransformResult) {
        AppDialogs.toast('No FHIR transformation result available. Please complete the transformation first.', 'warning');
        return;
    }

    const resources = data.fhirTransformResult.fhirResources || [];
    const bundle = data.fhirTransformResult.bundle || null;

    // ── Group resources by resourceType ─────────────────────────────────────
    // Order: single-instance types first, then multi-instance (e.g. Observation last)
    const groupMap = {};
    resources.forEach((r, i) => {
        const rt = r.resourceType || 'Resource';
        if (!groupMap[rt]) groupMap[rt] = [];
        groupMap[rt].push({ data: r, index: i });
    });
    if (bundle) groupMap['Bundle'] = [{ data: bundle, index: -1 }];
    const groupKeys = Object.keys(groupMap).sort((a, b) => {
        // Single-instance types sort before multi-instance
        const aLen = groupMap[a].length, bLen = groupMap[b].length;
        if (aLen !== bLen) return aLen - bLen;
        return a.localeCompare(b);
    });

    // ── Modal scaffold ───────────────────────────────────────────────────────
    const modal = document.createElement('div');
    modal.id = 'fhir-json-modal';
    modal.style.cssText = 'position:fixed;top:0;left:0;right:0;bottom:0;background:rgba(0,0,0,0.8);z-index:10000;display:flex;align-items:center;justify-content:center;padding:20px;';

    const inner = document.createElement('div');
    inner.style.cssText = 'background:#0f172a;border-radius:12px;width:100%;max-width:1300px;max-height:92vh;display:flex;flex-direction:column;gap:0;overflow:hidden;box-shadow:0 25px 50px rgba(0,0,0,0.6);';
    modal.appendChild(inner);

    // ── Header ───────────────────────────────────────────────────────────────
    const header = document.createElement('div');
    header.style.cssText = 'display:flex;justify-content:space-between;align-items:center;padding:14px 20px;background:#1e293b;border-bottom:1px solid #334155;flex-shrink:0;';
    const title = document.createElement('span');
    title.style.cssText = 'color:#e2e8f0;font-size:15px;font-weight:600;';
    title.textContent = 'FHIR Resources — ' + resources.length + ' resource' + (resources.length !== 1 ? 's' : '');
    header.appendChild(title);
    const btnBar = document.createElement('div');
    btnBar.style.cssText = 'display:flex;gap:8px;';
    [
        { label: 'Copy All', bg: '#6366f1', fn: 'window.copyFHIRJSON()' },
        { label: 'Download', bg: '#10b981', fn: 'window.downloadFHIRJSON()' },
        { label: '✕ Close',  bg: '#ef4444', fn: 'window.closeFHIRJSONViewer(this)' },
    ].forEach(({ label, bg, fn }) => {
        const b = document.createElement('button');
        b.textContent = label;
        b.setAttribute('onclick', fn);
        b.style.cssText = `padding:6px 14px;background:${bg};color:white;border:none;border-radius:6px;cursor:pointer;font-size:12px;font-weight:500;`;
        btnBar.appendChild(b);
    });
    header.appendChild(btnBar);
    inner.appendChild(header);

    // ── Type tabs (one per resourceType) ─────────────────────────────────────
    const typeTabBar = document.createElement('div');
    typeTabBar.style.cssText = 'display:flex;background:#1e293b;border-bottom:2px solid #334155;overflow-x:auto;flex-shrink:0;scrollbar-width:thin;';
    groupKeys.forEach((rt, idx) => {
        const count = groupMap[rt].length;
        const btn = document.createElement('button');
        btn.className = 'fhir-type-tab';
        btn.dataset.rt = rt;
        const badge = count > 1 ? ` <span style="background:#334155;border-radius:10px;padding:1px 7px;font-size:11px;">${count}</span>` : '';
        btn.innerHTML = `<span style="font-size:13px;">${rt}</span>${badge}`;
        const isFirst = idx === 0;
        btn.style.cssText = `padding:10px 18px;border:none;border-bottom:3px solid ${isFirst?'#3b82f6':'transparent'};background:transparent;color:${isFirst?'#e2e8f0':'#94a3b8'};cursor:pointer;white-space:nowrap;flex-shrink:0;font-weight:${isFirst?'600':'400'};transition:all 0.15s;`;
        btn.addEventListener('click', () => window._fhirSelectType(rt));
        typeTabBar.appendChild(btn);
    });
    inner.appendChild(typeTabBar);

    // ── Body: list pane + JSON pane ───────────────────────────────────────────
    const body = document.createElement('div');
    body.style.cssText = 'display:flex;flex:1;min-height:0;';
    inner.appendChild(body);

    // Left list pane
    const listPane = document.createElement('div');
    listPane.id = 'fhir-list-pane';
    listPane.style.cssText = 'width:200px;flex-shrink:0;background:#1e293b;border-right:1px solid #334155;overflow-y:auto;';
    body.appendChild(listPane);

    // Right JSON pane
    const jsonPane = document.createElement('div');
    jsonPane.id = 'fhir-json-pane';
    jsonPane.style.cssText = 'flex:1;overflow:auto;background:#1a1a2e;min-height:0;';
    body.appendChild(jsonPane);

    const pre = document.createElement('pre');
    pre.id = 'fhir-json-pre';
    pre.style.cssText = 'margin:0;padding:20px;font-family:\'Fira Code\',\'Courier New\',monospace;font-size:12.5px;line-height:1.65;color:#cdd6e0;min-height:100%;';
    jsonPane.appendChild(pre);

    document.body.appendChild(modal);

    // ── State ─────────────────────────────────────────────────────────────────
    window._fhirGroupMap = groupMap;
    window._fhirGroupKeys = groupKeys;
    window._fhirCurrentType = groupKeys[0];
    window._fhirCurrentIdx = 0;

    // Select the first type to populate list + JSON
    window._fhirSelectType(groupKeys[0]);
};

// Apply syntax highlighting to a <pre> element that already has textContent set.
// Works by reading the text, escaping HTML, then wrapping tokens in colored spans.
window._applyJSONHighlight = function(pre) {
    const text = pre.textContent;
    // Escape for safe innerHTML insertion
    let html = text
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;');
    // Colour tokens
    html = html.replace(
        /("(?:\\u[0-9a-fA-F]{4}|\\[^u]|[^\\"])*"(\s*:)?|\b(?:true|false|null)\b|-?\d+(?:\.\d*)?(?:[eE][+\-]?\d+)?)/g,
        function(match) {
            if (/^"/.test(match)) {
                return /:$/.test(match)
                    ? '<span style="color:#7dd3fc">' + match + '</span>'   // key — sky blue
                    : '<span style="color:#a5d6a7">' + match + '</span>';  // string value — green
            }
            if (/true|false/.test(match)) return '<span style="color:#7986cb">' + match + '</span>';  // bool — indigo
            if (/null/.test(match))       return '<span style="color:#ef9a9a">' + match + '</span>';  // null — red
            return '<span style="color:#ffcc80">' + match + '</span>';                                 // number — amber
        }
    );
    // Colour braces / brackets / commas
    html = html.replace(/([{}\[\]])/g, '<span style="color:#b0bec5">$1</span>');
    pre.style.color = '#cdd6e0';
    pre.innerHTML = html;
};

// Select a resource-type group — updates type tab highlight and rebuilds the list pane.
window._fhirSelectType = function(rt) {
    window._fhirCurrentType = rt;
    window._fhirCurrentIdx = 0;

    // Update type tab styles
    document.querySelectorAll('.fhir-type-tab').forEach(btn => {
        const active = btn.dataset.rt === rt;
        btn.style.borderBottomColor = active ? '#3b82f6' : 'transparent';
        btn.style.color = active ? '#e2e8f0' : '#94a3b8';
        btn.style.fontWeight = active ? '600' : '400';
    });

    // Rebuild list pane
    const listPane = document.getElementById('fhir-list-pane');
    if (!listPane) return;
    listPane.innerHTML = '';
    const items = window._fhirGroupMap[rt] || [];

    if (items.length === 1) {
        // Single item — skip list, just show JSON directly
        listPane.style.display = 'none';
    } else {
        listPane.style.display = 'block';
        items.forEach((item, i) => {
            const row = document.createElement('button');
            row.className = 'fhir-list-row';
            row.dataset.idx = i;
            // Label: use resource id if available, otherwise ordinal
            const idLabel = item.data.id ? item.data.id : '#' + (i + 1);
            row.style.cssText = `display:block;width:100%;text-align:left;padding:9px 14px;border:none;border-bottom:1px solid #334155;background:${i===0?'#1e3a5f':'transparent'};color:${i===0?'#93c5fd':'#94a3b8'};cursor:pointer;font-size:12px;font-family:inherit;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;`;
            row.textContent = idLabel;
            row.addEventListener('click', () => window._fhirSelectItem(i));
            listPane.appendChild(row);
        });
    }

    // Show JSON for first item
    window._fhirRenderJSON(items[0]?.data);
};

// Select a specific item within the current type group.
window._fhirSelectItem = function(idx) {
    window._fhirCurrentIdx = idx;
    const items = window._fhirGroupMap[window._fhirCurrentType] || [];

    // Update list row highlight
    document.querySelectorAll('.fhir-list-row').forEach(row => {
        const active = parseInt(row.dataset.idx) === idx;
        row.style.background = active ? '#1e3a5f' : 'transparent';
        row.style.color = active ? '#93c5fd' : '#94a3b8';
    });

    window._fhirRenderJSON(items[idx]?.data);
};

// Render a resource into the JSON pane with syntax highlighting.
window._fhirRenderJSON = function(resource) {
    const pre = document.getElementById('fhir-json-pre');
    if (!pre) return;
    pre.textContent = resource ? JSON.stringify(resource, null, 2) : '';
    window._applyJSONHighlight(pre);
};

// Legacy compatibility — still used by any code that calls switchFHIRTab directly.
window.switchFHIRTab = function(key) {
    // key may be 'r0', 'r1', 'bundle' from old callers — map to new system
    if (key === 'bundle') { window._fhirSelectType('Bundle'); return; }
    const i = parseInt(key.replace('r', ''), 10);
    if (!isNaN(i) && window._fhirGroupKeys) {
        // Find which type contains global index i
        let seen = 0;
        for (const rt of window._fhirGroupKeys) {
            const items = window._fhirGroupMap[rt];
            if (i < seen + items.length) {
                window._fhirSelectType(rt);
                window._fhirSelectItem(i - seen);
                return;
            }
            seen += items.length;
        }
    }
};

window.copyFHIRJSON = function() {
    const data = window.wizardController?.getCurrentData();
    if (data?.fhirTransformResult) {
        const json = JSON.stringify(data.fhirTransformResult.fhirResources, null, 2);
        navigator.clipboard.writeText(json).then(() => {
            AppDialogs.toast('FHIR JSON copied to clipboard!', 'success');
        });
    }
};

window.downloadFHIRJSON = function() {
    const data = window.wizardController?.getCurrentData();
    if (data?.fhirTransformResult) {
        const json = JSON.stringify(data.fhirTransformResult.fhirResources, null, 2);
        const blob = new Blob([json], { type: 'application/json' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `fhir-resources-${Date.now()}.json`;
        a.click();
        URL.revokeObjectURL(url);
    }
};

// syntaxHighlightJSON kept for backward compatibility (other callers if any)
window.syntaxHighlightJSON = function(obj) {
    const pre = document.createElement('pre');
    pre.textContent = JSON.stringify(obj, null, 2);
    window._applyJSONHighlight(pre);
    return pre.innerHTML;
};

// Close FHIR JSON viewer modal
window.closeFHIRJSONViewer = function(button) {
    console.log('🚪 Closing FHIR JSON viewer...');
    const modal = button.closest('[style*="position: fixed"]');
    if (modal && modal.parentNode) {
        modal.parentNode.removeChild(modal);
    }
};

window.WizardView = WizardView;

// ── Wizard Message Family Filter helpers ─────────────────────────────────────
window._wizardToggleFamilyFilter = function(enabled) {
    const picker = document.getElementById('wizardFamilyFilterPicker');
    const hidden = document.getElementById('wizardAcceptedMessageFamilies');
    if (picker) picker.style.display = enabled ? 'block' : 'none';
    if (!enabled) {
        if (hidden) hidden.value = '';
        document.querySelectorAll('#wizardFamilyChips .family-chip').forEach(c => c.classList.remove('selected'));
    }
};

window._wizardToggleFamilyChip = function(family) {
    const chip = document.querySelector(`#wizardFamilyChips [data-family="${family}"]`);
    const hidden = document.getElementById('wizardAcceptedMessageFamilies');
    if (!chip || !hidden) return;
    chip.classList.toggle('selected');
    let current = [];
    try { current = hidden.value ? JSON.parse(hidden.value) : []; } catch (e) {}
    const idx = current.indexOf(family);
    if (idx >= 0) current.splice(idx, 1); else current.push(family);
    hidden.value = current.length > 0 ? JSON.stringify(current) : '';
    // Keep the model in sync immediately
    if (window.wizardController?.model?.data) {
        window.wizardController.model.data.acceptedMessageFamilies = current.length > 0 ? current : null;
    }
};