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

        this.initializeView();
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
                                        <h2>HL7-FHIR</h2>
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
            { number: 2, title: 'HL7 Parsing', icon: '🔍', description: 'Message analysis' },
            { number: 3, title: 'FHIR Transform', icon: '🔄', description: 'HL7 to FHIR conversion' },
            { number: 4, title: 'Target Config', icon: '🎯', description: 'FHIR endpoint setup' }
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
                                    <option value="file" ${data.sourceType === 'file' ? 'selected' : ''}>File-based</option>
                                </select>
                            </div>
                            <div class="form-group">
                                <label for="sourceConnectivity" class="form-label required">Connectivity</label>
                                <select id="sourceConnectivity" class="form-control" required>
                                    <option value="tcp" ${data.sourceConnectivity === 'tcp' ? 'selected' : ''}>TCP/MLLP (Standard)</option>
                                    <option value="http" ${data.sourceConnectivity === 'http' ? 'selected' : ''}>HTTP/REST</option>
                                    <option value="file" ${data.sourceConnectivity === 'file' ? 'selected' : ''}>File Input</option>
                                </select>
                            </div>
                        </div>
                        <div id="sourceConfigPanel" class="config-panel">
                            ${this.getSourceConfigPanel(data.sourceConnectivity, data.sourceConfig, data.sourceType)}
                        </div>
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
                                        HL7 v2.x → FHIR R4 (Automatic Transformation)
                                    </option>
                                    <option value="ccd_to_fhir" ${data.transformationFlow === 'ccd_to_fhir' ? 'selected' : ''}>
                                        CCD/C-CDA → FHIR R4 (Automatic Transformation)
                                    </option>
                                    <option value="hl7_to_fhir_stu3" ${data.transformationFlow === 'hl7_to_fhir_stu3' ? 'selected' : ''}>
                                        HL7 v2.x → FHIR STU3 (Automatic Transformation)
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
                    <h3>HL7 Message Parsing & Sample</h3>
                    <p>Provide sample HL7 message for parsing and validation</p>
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
                                <button type="button" class="btn btn-primary" id="parseHL7Btn">
                                    <span class="btn-icon">🔍</span>
                                    Parse HL7 Message
                                </button>
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
                        <option value="STU3" ${config.fhirVersion === 'STU3' ? 'selected' : ''}>FHIR STU3</option>
                        <option value="DSTU2" ${config.fhirVersion === 'DSTU2' ? 'selected' : ''}>FHIR DSTU2 (Legacy)</option>
                    </select>
                    <div class="form-hint">Select the FHIR version your sender systems will use</div>
                </div>

                <!-- Supported Operations -->
                <div class="form-group">
                    <label class="form-label">
                        Supported REST Operations
                        <a href="#" class="help-link" onclick="event.preventDefault(); alert('Choose which FHIR REST API operations to enable:\\n\\n• CREATE (POST) - Most common, create new resources\\n• READ (GET) - Retrieve resources by ID\\n• UPDATE (PUT) - Replace entire resource\\n• PATCH - Partial updates\\n• DELETE - Remove resources (use with caution)\\n• SEARCH - Query with parameters\\n• BATCH/TRANSACTION - Multiple operations in one request\\n\\nRecommendation: Start with CREATE only, add others as needed.');" title="Click for help">ⓘ</a>
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
        return `
            <div class="wizard-step-content" data-step="4">
                <div class="wizard-step-header">
                    <h3>Target Configuration</h3>
                    <p>Configure where FHIR resources will be sent</p>
                </div>

                <div class="wizard-form">
                    <div class="form-row">
                        <div class="form-group">
                            <label for="targetType" class="form-label required">Target Format</label>
                            <select id="targetType" class="form-control" required>
                                <option value="fhir" ${data.targetType === 'fhir' ? 'selected' : ''}>FHIR R4 (Recommended)</option>
                                <option value="fhir-stu3" ${data.targetType === 'fhir-stu3' ? 'selected' : ''}>FHIR STU3</option>
                                <option value="hl7v2" ${data.targetType === 'hl7v2' ? 'selected' : ''}>HL7 v2.x</option>
                                <option value="database" ${data.targetType === 'database' ? 'selected' : ''}>Database</option>
                                <option value="file" ${data.targetType === 'file' ? 'selected' : ''}>File Output</option>
                            </select>
                            <div class="form-hint">💡 OOB: FHIR R4 is the current standard</div>
                        </div>

                        <div class="form-group">
                            <label for="targetConnectivity" class="form-label required">Connectivity</label>
                            <select id="targetConnectivity" class="form-control" required>
                                <option value="http" ${data.targetConnectivity === 'http' ? 'selected' : ''}>HTTP/REST (Standard)</option>
                                <option value="tcp" ${data.targetConnectivity === 'tcp' ? 'selected' : ''}>TCP/MLLP</option>
                                <option value="file" ${data.targetConnectivity === 'file' ? 'selected' : ''}>File Output</option>
                                <option value="database" ${data.targetConnectivity === 'database' ? 'selected' : ''}>Database</option>
                            </select>
                            <div class="form-hint">💡 OOB: HTTP/REST is FHIR standard</div>
                        </div>
                    </div>

                    <!-- Dynamic configuration based on connectivity -->
                    <div id="targetConfigPanel" class="config-panel">
                        ${this.getTargetConfigPanel(data.targetConnectivity, {...(data.targetConfig || {}), transformationFlow: data.transformationFlow})}
                    </div>

                    <!-- FHIR Server Testing -->
                    <div class="connection-test">
                        <button type="button" class="btn btn-outline-primary" id="testTargetConnection">
                            <span class="btn-icon">🎯</span>
                            Test FHIR Server
                        </button>
                        <div class="connection-status" id="targetConnectionStatus"></div>
                    </div>
                </div>
            </div>
        `;
    }

    /**
     * Check if delivery mode options should be shown based on selected transformation flow
     * Only show for automatic transformation flows, not for passthrough/receiver flows
     */
    shouldShowDeliveryMode(transformationFlow) {
        // Delivery mode is relevant for transformation flows that output FHIR
        const transformationFlowsWithDelivery = [
            'hl7_to_fhir',         // HL7 v2.x → FHIR R4
            'ccd_to_fhir',         // CCD/C-CDA → FHIR R4
            'hl7_to_fhir_stu3',    // HL7 v2.x → FHIR STU3
            // Easy to add: 'x12_to_fhir', 'csv_to_fhir', etc.
        ];

        return transformationFlowsWithDelivery.includes(transformationFlow);
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
                                    <option value="R4" ${config.version === 'R4' ? 'selected' : ''}>R4 (Recommended)</option>
                                    <option value="STU3" ${config.version === 'STU3' ? 'selected' : ''}>STU3</option>
                                    <option value="DSTU2" ${config.version === 'DSTU2' ? 'selected' : ''}>DSTU2</option>
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
                    <h3>🔄 HL7 to FHIR Mapping Configuration</h3>
                    <p>Review and customize field mappings between HL7 segments and FHIR resources</p>
                </div>

                <div class="wizard-form">
                    <!-- Mapping Configuration Interface -->
                    <div class="fhir-mapping-interface">
                        <!-- Configuration Header -->
                        <div class="mapping-config-header" style="background: linear-gradient(135deg, #f8f9ff 0%, #fff 100%); border: 2px solid #f8bbd9; border-radius: 12px; padding: 16px 20px; margin-bottom: 16px;">
                            <div style="display: flex; justify-content: space-between; align-items: center;">
                                <div>
                                    <h4 style="margin: 0 0 6px 0; font-size: 16px; font-weight: 600; color: #1e3a8a;">🗺️ Field Mapping Configuration</h4>
                                    <div style="display: flex; align-items: center; gap: 8px; font-size: 12px; color: #6b7280;">
                                        <span style="background: #e0e7ff; color: #1e3a8a; padding: 2px 8px; border-radius: 6px; font-weight: 500;" id="fhir-message-type">${data.detectedMessageType || data.parsedHL7Data?.messageType?.name || 'Loading...'}</span>
                                        <span style="color: #d1d5db;">•</span>
                                        <span style="background: #e0e7ff; color: #1e3a8a; padding: 2px 8px; border-radius: 6px; font-weight: 500;">FHIR R4</span>
                                        <span style="color: #d1d5db;">•</span>
                                        <span style="padding: 2px 8px; border-radius: 6px; font-weight: 500; font-size: 11px; text-transform: uppercase;" id="mapping-count-badge">
                                            <span id="mapping-count">${this.getMappingCount(data)}</span> Mappings
                                        </span>
                                    </div>
                                </div>
                                <div style="display: flex; gap: 12px;">
                                    <button style="padding: 8px 16px; border: 2px solid #6366f1; border-radius: 6px; font-size: 13px; font-weight: 500; cursor: pointer; display: flex; align-items: center; gap: 6px; background: white; color: #6366f1; transition: all 0.2s ease;"
                                            id="btn-view-fhir-json" onclick="window.viewRawFHIRJSON()">
                                        📄 View FHIR JSON
                                    </button>
                                    <button style="padding: 8px 16px; border: 2px solid #10b981; border-radius: 6px; font-size: 13px; font-weight: 500; cursor: pointer; display: flex; align-items: center; gap: 6px; background: white; color: #10b981; transition: all 0.2s ease;"
                                            id="btn-save-mappings" onclick="window.saveFHIRMappingConfiguration()">
                                        💾 Save Configuration
                                    </button>
                                    <button style="padding: 8px 16px; border: none; border-radius: 6px; font-size: 13px; font-weight: 500; cursor: pointer; display: flex; align-items: center; gap: 6px; background: #1e3a8a; color: white; transition: all 0.2s ease;"
                                            id="btn-test-mappings" onclick="window.testFHIRMappings()">
                                        🧪 Test Transform
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
                                    <option value="Patient">Patient</option>
                                    <option value="Encounter">Encounter</option>
                                    <option value="MessageHeader">MessageHeader</option>
                                    <option value="Observation">Observation</option>
                                </select>
                            </div>
                        </div>

                        <!-- Mapping Results Container -->
                        <div id="fhir-mapping-container" style="min-height: 400px; border: 2px solid #e2e8f0; border-radius: 12px; background: white;">
                            ${this.getFHIRMappingContent(data)}
                        </div>

                        <!-- Advanced Options Panel (Collapsible) -->
                        <div style="margin-top: 16px;">
                            <button id="toggle-advanced-options" style="width: 100%; padding: 12px; border: 2px solid #f3f4f6; border-radius: 8px; background: #fafafa; color: #374151; font-size: 14px; font-weight: 500; cursor: pointer; display: flex; justify-content: space-between; align-items: center;">
                                <span>🔧 Advanced Mapping Options</span>
                                <span id="advanced-toggle-icon">▼</span>
                            </button>
                            <div id="advanced-options-panel" style="display: none; margin-top: 8px; border: 2px solid #f3f4f6; border-radius: 8px; background: white; padding: 16px;">
                                <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(250px, 1fr)); gap: 16px;">
                                    <div>
                                        <label style="display: block; font-size: 13px; font-weight: 500; color: #374151; margin-bottom: 4px;">Data Transformation Rules</label>
                                        <select id="transformation-preset" style="width: 100%; padding: 8px; border: 1px solid #d1d5db; border-radius: 4px; font-size: 13px;">
                                            <option value="standard">Standard HL7 → FHIR</option>
                                            <option value="minimal">Minimal Required Fields</option>
                                            <option value="comprehensive">Comprehensive Mapping</option>
                                            <option value="custom">Custom Rules</option>
                                        </select>
                                    </div>
                                    <div>
                                        <label style="display: block; font-size: 13px; font-weight: 500; color: #374151; margin-bottom: 4px;">Validation Level</label>
                                        <select id="validation-level" style="width: 100%; padding: 8px; border: 1px solid #d1d5db; border-radius: 4px; font-size: 13px;">
                                            <option value="strict">Strict FHIR Compliance</option>
                                            <option value="moderate">Moderate (Recommended)</option>
                                            <option value="lenient">Lenient</option>
                                        </select>
                                    </div>
                                    <div>
                                        <label style="display: block; font-size: 13px; font-weight: 500; color: #374151; margin-bottom: 4px;">Missing Field Handling</label>
                                        <select id="missing-field-handling" style="width: 100%; padding: 8px; border: 1px solid #d1d5db; border-radius: 4px; font-size: 13px;">
                                            <option value="skip">Skip Missing Fields</option>
                                            <option value="default">Use Default Values</option>
                                            <option value="error">Report as Error</option>
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
        const resources = ['Patient', 'Encounter', 'MessageHeader', 'Observation'];

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
                        ${mappings.slice(0, 12).map((mapping, index) => `
                            <div style="padding: 12px 16px; border-bottom: 1px solid #f1f5f9; display: flex; justify-content: space-between; align-items: center;">
                                <div style="flex: 1; font-family: monospace;">
                                    <div style="font-size: 13px; font-weight: 600; color: #1e293b;">${mapping.hl7Path || mapping.hl7Field}</div>
                                    <div style="font-size: 11px; color: #64748b; margin-top: 2px;">"${(mapping.hl7Value || '').substring(0, 30)}${(mapping.hl7Value || '').length > 30 ? '...' : ''}"</div>
                                </div>
                                <div style="margin: 0 16px; color: #6b7280; font-weight: bold;">→</div>
                                <div style="flex: 1; text-align: right; font-family: monospace;">
                                    <div style="font-size: 13px; font-weight: 600; color: #1e40af;">${mapping.fhirPath || mapping.fhirField}</div>
                                    <div style="font-size: 11px; color: #64748b; margin-top: 2px;">"${(mapping.fhirValue || mapping.transformedValue || '').substring(0, 30)}${(mapping.fhirValue || mapping.transformedValue || '').length > 30 ? '...' : ''}"</div>
                                </div>
                            </div>
                        `).join('')}
                        ${mappings.length > 12 ? `
                            <div style="padding: 12px 16px; text-align: center; color: #6b7280; font-size: 12px; font-style: italic;">
                                ... and ${mappings.length - 12} more mappings
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
                    ${data.detectedMessageType ? `Configure how HL7 ${data.detectedMessageType} fields map to FHIR R4 resources` : 'Parse an HL7 message first to configure field mappings'}
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
                <div style="border: 2px dashed #d1d5db; border-radius: 12px; padding: 16px; text-align: center; color: #6b7280;">
                    <div style="font-size: 32px; margin-bottom: 8px;">👤</div>
                    <div style="font-size: 14px; font-weight: 500;">No ${resourceType} mappings found</div>
                    <button onclick="window.addResourceMapping('${resourceType}')"
                            style="margin-top: 8px; padding: 6px 12px; background: #1e3a8a; color: white; border: none; border-radius: 4px; font-size: 12px; cursor: pointer;">
                        ➕ Add Mapping
                    </button>
                </div>
            `;
        }

        return `
            <div style="border: 2px solid ${hasIssues ? '#fecaca' : '#e0e7ff'}; border-radius: 12px; background: white; overflow: hidden;">
                <!-- Resource Header -->
                <div style="background: ${hasIssues ? '#fef2f2' : 'linear-gradient(135deg, #1e3a8a 0%, #1e40af 100%)'}; color: ${hasIssues ? '#dc2626' : 'white'}; padding: 12px 16px; display: flex; justify-content: space-between; align-items: center;">
                    <div>
                        <h5 style="margin: 0; font-size: 14px; font-weight: 600;">${this.getResourceIcon(resourceType)} ${resourceType} Resource</h5>
                        <div style="font-size: 12px; opacity: 0.9; margin-top: 2px;">${resourceMappings.length} field mappings</div>
                    </div>
                    <div style="display: flex; gap: 8px;">
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
        const transformType = mapping.transformType || 'direct';
        const isRequired = mapping.isRequired ? '⭐' : '';

        // Extract actual values from parsed HL7 data and FHIR resources
        const hl7Value = this.extractHL7Value(hl7Field);
        const fhirValue = this.extractFHIRValue(fhirField, resourceType);

        return `
            <div style="padding: 12px 16px; border-bottom: 1px solid #f1f5f9; display: flex; align-items: center; gap: 16px; ${hasIssue ? 'background: #fef2f2;' : ''}"
                 data-mapping-index="${index}" data-resource="${resourceType}">

                <!-- HL7 Source -->
                <div style="flex: 1; min-width: 0;">
                    <div style="display: flex; align-items: center; gap: 8px; margin-bottom: 4px;">
                        <input type="text" value="${hl7Field}"
                               onchange="window.updateMapping(${index}, 'hl7Field', this.value)"
                               style="flex: 1; padding: 4px 8px; border: 1px solid #d1d5db; border-radius: 4px; font-size: 12px; font-family: monospace; background: #f9fafb;"
                               readonly>
                        ${hasIssue ? '<span style="color: #dc2626; font-size: 12px;">⚠️</span>' : ''}
                    </div>
                    <div style="font-size: 11px; color: #6b7280; padding: 2px 4px; background: #f3f4f6; border-radius: 4px; font-family: monospace; max-width: 200px; overflow: hidden; text-overflow: ellipsis;">
                        "${hl7Value.substring(0, 25)}${hl7Value.length > 25 ? '...' : ''}"
                    </div>
                </div>

                <!-- Transformation Arrow & Options -->
                <div style="display: flex; flex-direction: column; align-items: center; gap: 4px;">
                    <div style="color: #6b7280; font-weight: bold; font-size: 14px;">→</div>
                    <select onchange="window.updateMappingTransformation(${index}, this.value)"
                            style="padding: 2px 4px; border: 1px solid #d1d5db; border-radius: 4px; font-size: 10px; min-width: 80px;">
                        <option value="direct" ${(mapping.transform || 'direct') === 'direct' ? 'selected' : ''}>Direct</option>
                        <option value="lookup" ${(mapping.transform || '') === 'lookup' ? 'selected' : ''}>Lookup</option>
                        <option value="format" ${(mapping.transform || '') === 'format' ? 'selected' : ''}>Format</option>
                        <option value="custom" ${(mapping.transform || '') === 'custom' ? 'selected' : ''}>Custom</option>
                    </select>
                </div>

                <!-- FHIR Target -->
                <div style="flex: 1; min-width: 0;">
                    <div style="margin-bottom: 4px;">
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

                <!-- Actions -->
                <div style="display: flex; gap: 4px;">
                    <button onclick="window.editMappingDetails(${index})"
                            style="padding: 4px 8px; background: #f3f4f6; color: #374151; border: 1px solid #d1d5db; border-radius: 4px; font-size: 11px; cursor: pointer;" title="Edit Details">
                        ✏️
                    </button>
                    <button onclick="window.deleteMappingRow(${index})"
                            style="padding: 4px 8px; background: #fef2f2; color: #dc2626; border: 1px solid #fecaca; border-radius: 4px; font-size: 11px; cursor: pointer;" title="Delete">
                        🗑️
                    </button>
                </div>
            </div>
        `;
    }

    /**
     * Get expected FHIR resources for a message type
     */
    getExpectedResourcesForMessageType(messageType) {
        const resourceMap = {
            'ADT^A01': [
                { name: 'Patient', icon: '👤', color: '#f0fdf4', borderColor: '#bbf7d0', textColor: '#15803d' },
                { name: 'Encounter', icon: '🏥', color: '#eff6ff', borderColor: '#bfdbfe', textColor: '#1d4ed8' },
                { name: 'MessageHeader', icon: '📨', color: '#fdf2f8', borderColor: '#fbcfe8', textColor: '#be185d' }
            ],
            'ORU^R01': [
                { name: 'Patient', icon: '👤', color: '#f0fdf4', borderColor: '#bbf7d0', textColor: '#15803d' },
                { name: 'Observation', icon: '🔬', color: '#fdf2f8', borderColor: '#fbcfe8', textColor: '#be185d' },
                { name: 'DiagnosticReport', icon: '📊', color: '#eff6ff', borderColor: '#bfdbfe', textColor: '#1d4ed8' }
            ]
        };
        return resourceMap[messageType] || [
            { name: 'Patient', icon: '👤', color: '#f0fdf4', borderColor: '#bbf7d0', textColor: '#15803d' },
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
                                <span>FHIR R4 compliant</span>
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

                <!-- Field Mappings Summary -->
                ${atomicMappings.length > 0 ? `
                    <div style="background: white; border: 2px solid #f8bbd9; border-radius: 12px; overflow: hidden;">
                        <div style="background: linear-gradient(135deg, #1e3a8a 0%, #1e40af 100%); color: white; padding: 16px;">
                            <h5 style="margin: 0; font-size: 16px; font-weight: 600;">🔗 Field Mappings Applied</h5>
                            <p style="margin: 4px 0 0 0; font-size: 14px; opacity: 0.9;">${atomicMappings.length} HL7 fields successfully mapped to FHIR elements</p>
                        </div>
                        <div style="max-height: 300px; overflow-y: auto;">
                            ${atomicMappings.slice(0, 8).map(mapping => `
                                <div style="padding: 12px 16px; border-bottom: 1px solid #f1f5f9; display: flex; justify-content: space-between; align-items: center;">
                                    <div style="flex: 1; font-family: monospace;">
                                        <div style="font-size: 13px; font-weight: 600; color: #1e293b;">${mapping.hl7Path}</div>
                                        <div style="font-size: 11px; color: #64748b; margin-top: 2px;">"${(mapping.hl7Value || '').substring(0, 30)}${(mapping.hl7Value || '').length > 30 ? '...' : ''}"</div>
                                    </div>
                                    <div style="margin: 0 16px; color: #6b7280; font-weight: bold;">→</div>
                                    <div style="flex: 1; text-align: right; font-family: monospace;">
                                        <div style="font-size: 13px; font-weight: 600; color: #1e40af;">${mapping.fhirPath}</div>
                                        <div style="font-size: 11px; color: #64748b; margin-top: 2px;">"${(mapping.fhirValue || '').substring(0, 30)}${(mapping.fhirValue || '').length > 30 ? '...' : ''}"</div>
                                    </div>
                                </div>
                            `).join('')}
                            ${atomicMappings.length > 8 ? `
                                <div style="padding: 12px 16px; text-align: center; color: #6b7280; font-size: 12px; font-style: italic;">
                                    ... and ${atomicMappings.length - 8} more mappings
                                </div>
                            ` : ''}
                        </div>
                    </div>
                ` : ''}
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
                    <h3>HL7 to FHIR Mapping</h3>
                    <p>Configure how HL7 messages are transformed to FHIR resources</p>
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
        if (!data.parsedHL7Data?.enhancedSegments) {
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

        const segments = this.getSegmentsInMessageOrder(data.parsedHL7Data.enhancedSegments, data.parsedHL7Data);
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
        const isExpanded = this.expandedSegments.has(segName);
        const segmentErrors = validationErrors.filter(err => err.segment === segName);
        const hasIssues = segmentErrors.length > 0;

        // Get specific missing required fields for this segment
        const missingRequiredFields = segmentErrors.filter(err =>
            err.code === 'MISSING_REQUIRED_FIELD' || err.code === 'EMPTY_REQUIRED_FIELD'
        );

        const keyFields = this.getKeyFields(segName, segment);

        return `
            <div class="segment-compact ${hasIssues ? 'has-issues' : ''} ${isExpanded ? 'expanded' : ''} ${missingRequiredFields.length > 0 ? 'has-missing-required' : ''}">
                <div class="segment-row" onclick="toggleSegment('${segName}')">
                    <div class="segment-info">
                        <div class="segment-name-badge">
                            <span class="seg-name">${segName}</span>
                            ${this.renderDynamicBadges(segName, segment, segmentErrors)}
                            ${this.renderRequiredFieldsBadge(missingRequiredFields)}
                        </div>
                        <div class="segment-summary">
                            <span class="seg-desc">${this.truncateText(segment.description || `${segName} Segment`, 40)}</span>
                            ${keyFields.length > 0 ? `<span class="key-values">${keyFields.join(' • ')}</span>` : ''}
                        </div>
                    </div>
                    <div class="segment-meta">
                        <span class="field-count">${segment.fieldCount || segment.fields?.length || 0}</span>
                        <span class="expand-icon ${isExpanded ? 'expanded' : ''}">${isExpanded ? '−' : '+'}</span>
                    </div>
                </div>

                ${isExpanded ? this.renderSegmentFields(segName, segment, validationErrors) : ''}
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
                            <p class="text-muted">Add mappings to transform HL7 to FHIR</p>
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
                console.log('🔄 Source type changed:', e.target.value);
                this.updateSourceConfigPanel(container);

                this.dispatchEvent(new CustomEvent('fieldChange', {
                    detail: { field: 'sourceType', value: e.target.value }
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

        // Source Connectivity selector - re-render panel when changed
        const sourceConnectivitySelect = container.querySelector('#sourceConnectivity');
        if (sourceConnectivitySelect) {
            sourceConnectivitySelect.addEventListener('change', (e) => {
                console.log('🔄 Source connectivity changed:', e.target.value);
                this.updateSourceConfigPanel(container);

                this.dispatchEvent(new CustomEvent('fieldChange', {
                    detail: { field: 'sourceConnectivity', value: e.target.value }
                }));
            });
            console.log('✅ Source connectivity selector listener attached');
        }

        // HTTP Authentication Type selector - update auth details panel dynamically
        const httpAuthTypeSelect = container.querySelector('#httpAuthType');
        if (httpAuthTypeSelect) {
            httpAuthTypeSelect.addEventListener('change', (e) => {
                const authType = e.target.value;
                console.log('🔐 HTTP auth type changed:', authType);

                const authDetailsPanel = container.querySelector('#httpAuthDetails');
                if (authDetailsPanel) {
                    authDetailsPanel.innerHTML = this.getHttpAuthDetailsPanel(authType, {});
                }

                this.dispatchEvent(new CustomEvent('fieldChange', {
                    detail: { field: 'httpAuthType', value: authType }
                }));
            });
            console.log('✅ HTTP auth type selector listener attached');
        }

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
                        'ccd_to_fhir': {
                            title: '🔀 CCD/C-CDA → FHIR R4 Transformation',
                            text: 'Automatically converts C-CDA (Consolidated Clinical Document Architecture) documents to FHIR R4 resources. Ideal for EHR interoperability.'
                        },
                        'hl7_to_fhir_stu3': {
                            title: '🔀 HL7 v2.x → FHIR STU3 Transformation',
                            text: 'Automatically converts HL7 v2.x messages to FHIR STU3 resources for systems that haven\'t upgraded to R4.'
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

                this.dispatchEvent(new CustomEvent('parseHL7Requested', {
                    detail: { message: messageContent }
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
            btn.addEventListener('click', (e) => {
                const messageType = e.target.getAttribute('data-message-type');
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
                window.switchMappingView(view);
            });
        });

        // Resource filter dropdown
        const resourceFilter = container.querySelector('#resource-filter');
        if (resourceFilter) {
            resourceFilter.addEventListener('change', (e) => {
                const filter = e.target.value;
                console.log('🔍 Filtering resources:', filter);

                const groups = container.querySelectorAll('[id^="resource-mappings-"]');
                groups.forEach(group => {
                    const parent = group.closest('[style*="border: 2px"]');
                    if (parent) {
                        if (filter === 'all') {
                            parent.style.display = 'block';
                        } else {
                            const resourceType = group.id.replace('resource-mappings-', '');
                            parent.style.display = resourceType === filter ? 'block' : 'none';
                        }
                    }
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

                this.dispatchEvent(new CustomEvent('parseHL7Requested', {
                    detail: { message: messageContent }
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
            btn.addEventListener('click', (e) => {
                const messageType = e.target.getAttribute('data-message-type');
                this.dispatchEvent(new CustomEvent('sampleHL7Requested', {
                    detail: { messageType }
                }));
            });
        });

        // Segment toggle functionality
        this.setupSegmentToggles(container);

        // Setup target configuration listeners (delivery mode, authentication, resources)
        this.setupTargetConfigListeners(container);
        console.log('✅ Step 4 target config listeners attached');
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
     */
    syncFormDataToModel() {
        console.log('🔄 Syncing form data to model...');

        const data = {};

        // Step 1: Interface Basic Info (CRITICAL - must be preserved!)
        const interfaceName = this.container.querySelector('#interfaceName')?.value;
        const interfaceDescription = this.container.querySelector('#interfaceDescription')?.value;
        if (interfaceName !== undefined) {
            data.name = interfaceName || '';
            console.log('💾 Captured name:', data.name);
        }
        if (interfaceDescription !== undefined) {
            data.description = interfaceDescription || '';
            console.log('💾 Captured description:', data.description);
        }

        // Step 1: Source Configuration (CRITICAL for FHIR skip logic!)
        const sourceType = this.container.querySelector('#sourceType')?.value;
        const sourceConnectivity = this.container.querySelector('#sourceConnectivity')?.value;
        const sourcePort = this.container.querySelector('#sourcePort')?.value;
        const sourceHost = this.container.querySelector('#sourceHost')?.value;
        const transformationFlow = this.container.querySelector('#transformationFlow')?.value;

        if (sourceType !== undefined) {
            data.sourceType = sourceType;
            console.log('💾 Captured sourceType:', data.sourceType);
        }
        if (sourceConnectivity !== undefined) {
            data.sourceConnectivity = sourceConnectivity;
            console.log('💾 Captured sourceConnectivity:', data.sourceConnectivity);
        }
        if (transformationFlow !== undefined) {
            data.transformationFlow = transformationFlow;
            console.log('💾 Captured transformationFlow:', data.transformationFlow);
        }
        if (sourcePort !== undefined || sourceHost !== undefined) {
            data.sourceConfig = data.sourceConfig || {};
            if (sourcePort) data.sourceConfig.port = parseInt(sourcePort);
            if (sourceHost) data.sourceConfig.host = sourceHost;
        }

        // FHIR Receiver Configuration (comprehensive fields)
        if (sourceType === 'fhir' && sourceConnectivity === 'http') {
            data.sourceConfig = data.sourceConfig || {};

            // FHIR Receiver Base Configuration
            const fhirBasePath = this.container.querySelector('#fhirBasePath')?.value;
            const fhirVersion = this.container.querySelector('#fhirVersion')?.value;
            const fhirContentType = this.container.querySelector('#fhirContentType')?.value;

            if (fhirBasePath) data.sourceConfig.basePath = fhirBasePath;
            if (fhirVersion) data.sourceConfig.fhirVersion = fhirVersion;
            if (fhirContentType) data.sourceConfig.contentType = fhirContentType;

            // FHIR REST Operations
            const operations = [];
            ['CREATE', 'READ', 'UPDATE', 'PATCH', 'DELETE', 'SEARCH', 'BATCH'].forEach(op => {
                const checkbox = this.container.querySelector(`#fhirOperation${op}`);
                if (checkbox?.checked) operations.push(op);
            });
            if (operations.length > 0) data.sourceConfig.operations = operations;

            // HTTP Authentication
            const httpAuthType = this.container.querySelector('#httpAuthType')?.value;
            if (httpAuthType) {
                data.sourceConfig.authType = httpAuthType;

                // Auth type specific fields
                if (httpAuthType === 'api_key') {
                    data.sourceConfig.apiKeyHeader = this.container.querySelector('#authApiKeyHeader')?.value;
                    data.sourceConfig.apiKeyValue = this.container.querySelector('#authApiKeyValue')?.value;
                    data.sourceConfig.apiKeyLocation = this.container.querySelector('#authApiKeyLocation')?.value;
                } else if (httpAuthType === 'basic') {
                    data.sourceConfig.basicUsername = this.container.querySelector('#authBasicUsername')?.value;
                    data.sourceConfig.basicPassword = this.container.querySelector('#authBasicPassword')?.value;
                    data.sourceConfig.basicRealm = this.container.querySelector('#authBasicRealm')?.value;
                } else if (httpAuthType === 'bearer') {
                    data.sourceConfig.bearerToken = this.container.querySelector('#authBearerToken')?.value;
                    data.sourceConfig.bearerTokenValidation = this.container.querySelector('#authBearerTokenValidation')?.checked;
                } else if (httpAuthType === 'oauth2') {
                    data.sourceConfig.oauthIssuer = this.container.querySelector('#authOAuthIssuer')?.value;
                    data.sourceConfig.oauthAudience = this.container.querySelector('#authOAuthAudience')?.value;
                    data.sourceConfig.oauthScopes = this.container.querySelector('#authOAuthScopes')?.value;
                    data.sourceConfig.oauthClientId = this.container.querySelector('#authOAuthClientId')?.value;
                    data.sourceConfig.oauthClientSecret = this.container.querySelector('#authOAuthClientSecret')?.value;
                    data.sourceConfig.smartOnFhir = this.container.querySelector('#authSmartOnFhir')?.checked;
                } else if (httpAuthType === 'mtls') {
                    data.sourceConfig.mtlsServerCert = this.container.querySelector('#authMtlsServerCert')?.value;
                    data.sourceConfig.mtlsServerKey = this.container.querySelector('#authMtlsServerKey')?.value;
                    data.sourceConfig.mtlsClientCA = this.container.querySelector('#authMtlsClientCA')?.value;
                    data.sourceConfig.mtlsVerifyClient = this.container.querySelector('#authMtlsVerifyClient')?.checked;
                }
            }

            // Resource Filtering
            const acceptedResources = [];
            const resourceCheckboxes = this.container.querySelectorAll('input[name="fhirResource"]:checked');
            resourceCheckboxes.forEach(cb => acceptedResources.push(cb.value));
            if (acceptedResources.length > 0) data.sourceConfig.acceptedResources = acceptedResources;

            // Validation Settings
            const validateStructure = this.container.querySelector('#fhirValidateStructure')?.checked;
            const validateProfiles = this.container.querySelector('#fhirValidateProfiles')?.checked;
            const validateTerminology = this.container.querySelector('#fhirValidateTerminology')?.checked;
            if (validateStructure !== undefined) data.sourceConfig.validateStructure = validateStructure;
            if (validateProfiles !== undefined) data.sourceConfig.validateProfiles = validateProfiles;
            if (validateTerminology !== undefined) data.sourceConfig.validateTerminology = validateTerminology;

            // Rate Limiting
            const rateLimitEnabled = this.container.querySelector('#fhirRateLimitEnabled')?.checked;
            const rateLimitRequests = this.container.querySelector('#fhirRateLimitRequests')?.value;
            const rateLimitWindow = this.container.querySelector('#fhirRateLimitWindow')?.value;
            if (rateLimitEnabled !== undefined) data.sourceConfig.rateLimitEnabled = rateLimitEnabled;
            if (rateLimitRequests) data.sourceConfig.rateLimitRequests = parseInt(rateLimitRequests);
            if (rateLimitWindow) data.sourceConfig.rateLimitWindow = parseInt(rateLimitWindow);

            // Post-Reception Actions
            const postReceptionActions = [];
            ['store', 'transform', 'forward', 'workflow', 'audit'].forEach(action => {
                const checkbox = this.container.querySelector(`#fhirAction${action.charAt(0).toUpperCase() + action.slice(1)}`);
                if (checkbox?.checked) postReceptionActions.push(action);
            });
            if (postReceptionActions.length > 0) data.sourceConfig.postReceptionActions = postReceptionActions;

            console.log('💾 Captured FHIR receiver configuration:', data.sourceConfig);
        }

        // Step 4: Target Configuration
        const targetType = this.container.querySelector('#targetType')?.value;
        const targetConnectivity = this.container.querySelector('#targetConnectivity')?.value;
        const targetEndpoint = this.container.querySelector('#targetEndpoint')?.value;
        if (targetType !== undefined) data.targetType = targetType;
        if (targetConnectivity !== undefined) data.targetConnectivity = targetConnectivity;

        // Capture all target config fields
        if (targetEndpoint !== undefined) {
            data.targetConfig = data.targetConfig || {};
            data.targetConfig.endpoint = targetEndpoint;

            // Delivery mode
            const deliveryModeBundle = this.container.querySelector('#deliveryModeBundle');
            const deliveryModeIndividual = this.container.querySelector('#deliveryModeIndividual');
            if (deliveryModeBundle || deliveryModeIndividual) {
                data.targetConfig.deliveryMode = deliveryModeIndividual?.checked ? 'individual' : 'bundle';
            }

            // Individual resources (only if individual mode selected)
            if (deliveryModeIndividual?.checked) {
                const resourceCheckboxes = this.container.querySelectorAll('input[name="individualResources"]:checked');
                data.targetConfig.individualResources = Array.from(resourceCheckboxes).map(cb => cb.value);
            }

            // Default FHIR Operation
            const defaultOperation = this.container.querySelector('#defaultOperation')?.value;
            if (defaultOperation) {
                data.targetConfig.defaultOperation = defaultOperation;
            }

            // Authentication
            const authEnabled = this.container.querySelector('#targetAuthEnabled')?.checked;
            data.targetConfig.authEnabled = authEnabled;

            if (authEnabled) {
                const authType = this.container.querySelector('#authType')?.value;
                data.targetConfig.authType = authType;

                if (authType === 'basic') {
                    data.targetConfig.authUsername = this.container.querySelector('#authUsername')?.value;
                    data.targetConfig.authPassword = this.container.querySelector('#authPassword')?.value;
                } else if (authType === 'bearer') {
                    data.targetConfig.authToken = this.container.querySelector('#authToken')?.value;
                } else if (authType === 'oauth2') {
                    data.targetConfig.authClientId = this.container.querySelector('#authClientId')?.value;
                    data.targetConfig.authClientSecret = this.container.querySelector('#authClientSecret')?.value;
                    data.targetConfig.authTokenUrl = this.container.querySelector('#authTokenUrl')?.value;
                }
            }
        }

        console.log('📝 Synced form values:', data);

        // Update model data (dispatch event for async handling)
        if (Object.keys(data).length > 0) {
            this.dispatchEvent(new CustomEvent('dataSync', {
                detail: data,
                bubbles: true,  // Ensure event bubbles up
                composed: true   // Allow crossing shadow DOM boundaries
            }));
        }

        // Return the data so it can be used synchronously
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

        // Check basic requirements
        const hasValidName = name && name.length >= 3;

        // Check for duplicate (if we've already checked this name)
        const noDuplicate = !this.isDuplicateName;

        return hasValidName && noDuplicate;
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
        // Step 4 is target configuration - validate target settings
        const targetType = this.container.querySelector('#targetType')?.value;
        const targetConnectivity = this.container.querySelector('#targetConnectivity')?.value;

        // For HTTP connectivity, validate endpoint
        if (targetConnectivity === 'http') {
            const endpoint = this.container.querySelector('#targetEndpoint')?.value?.trim();
            try {
                if (endpoint) new URL(endpoint);
                return targetType && targetConnectivity && endpoint;
            } catch {
                return false;
            }
        }

        // For other connectivity types, just need type and connectivity
        return targetType && targetConnectivity;
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

// Global functions for Step 4 FHIR Mapping functionality
window.saveFHIRMappingConfiguration = function() {
    console.log('💾 Saving FHIR mapping configuration...');

    // Collect all mapping configurations from the UI
    const mappingRows = document.querySelectorAll('.fhir-mapping-row');
    const customMappings = [];

    mappingRows.forEach(row => {
        const hl7Field = row.querySelector('.hl7-source-select')?.value;
        const fhirField = row.querySelector('.fhir-target-select')?.value;
        const transformation = row.querySelector('.transformation-select')?.value || 'direct';
        const confidence = parseFloat(row.querySelector('.confidence-badge')?.textContent) || 0.85;

        if (hl7Field && fhirField) {
            customMappings.push({
                hl7Path: hl7Field,
                fhirPath: fhirField,
                transformation: transformation,
                confidence: confidence,
                enabled: !row.classList.contains('disabled')
            });
        }
    });

    // Get current wizard data
    const wizardData = window.wizardController?.getCurrentData() || {};

    // Update wizard data with custom mappings
    wizardData.fhirMappings = customMappings;
    wizardData.mappingConfiguration = {
        customMappings: customMappings,
        validationLevel: document.querySelector('input[name="validationLevel"]:checked')?.value || 'strict',
        transformationPreset: document.querySelector('select[name="transformationPreset"]')?.value || 'standard',
        savedAt: new Date().toISOString()
    };

    // Save to wizard controller
    if (window.wizardController) {
        window.wizardController.updateData(wizardData);
        console.log('✅ Mapping configuration saved:', customMappings.length, 'mappings');

        // Show success message
        showNotification('Configuration saved successfully!', 'success');

        // Move to next step if available
        if (window.wizardController.canProceedToNextStep) {
            setTimeout(() => {
                window.wizardController.goToStep(4); // Step 5 (Summary)
            }, 1500);
        }
    } else {
        console.error('❌ Wizard controller not available');
        showNotification('Error: Could not save configuration', 'error');
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
    console.log('👁️ Switching mapping view to:', viewType);

    // Update active button
    document.querySelectorAll('.mapping-view-btn').forEach(btn => {
        btn.classList.remove('active');
        if (btn.textContent.toLowerCase().includes(viewType.toLowerCase())) {
            btn.classList.add('active');
        }
    });

    // Find all mapping containers (resource groups and individual rows)
    const resourceGroups = document.querySelectorAll('[id^="resource-mappings-"]');
    const mappingRows = document.querySelectorAll('[style*="border: 2px solid #e5e7eb"]'); // Individual mapping rows

    let visibleCount = 0;

    switch (viewType) {
        case 'resource':
        case 'list':
            // Show grouped by resource (default view)
            resourceGroups.forEach(group => {
                const parent = group.closest('[style*="border: 2px"]');
                if (parent) {
                    parent.style.display = 'block';
                    visibleCount++;
                }
            });
            console.log('✅ Showing resource-grouped view');
            break;

        case 'all':
            // Show all mappings in flat list
            mappingRows.forEach(row => {
                row.style.display = 'block';
                visibleCount++;
            });
            console.log('✅ Showing all mappings view');
            break;

        case 'validation':
        case 'issues':
            // Show only mappings with issues
            const data = window.wizardController?.getCurrentData();
            const mappings = data?.fhirTransformResult?.atomicMappings || [];

            // Look for mappings with validation issues
            let issueCount = 0;
            mappingRows.forEach((row, index) => {
                const mapping = mappings[index];
                const hasIssue = mapping?.validationIssues?.length > 0 ||
                                mapping?.isRequired === false ||
                                mapping?.confidence < 0.8;

                row.style.display = hasIssue ? 'block' : 'none';
                if (hasIssue) issueCount++;
            });

            visibleCount = issueCount;
            console.log(`✅ Showing ${issueCount} mappings with validation issues`);

            if (issueCount === 0) {
                showNotification('No validation issues found - all mappings look good!', 'success');
            }
            break;
    }

    console.log(`✅ View switched to ${viewType}, showing ${visibleCount} items`);
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

window.deleteMappingRow = function(index) {
    console.log('🗑️ Deleting mapping row:', index);

    if (confirm('Are you sure you want to delete this mapping?')) {
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
                if (window.wizardFHIRTransform) {
                    console.log('🔄 Using existing transformation service');
                    await window.wizardFHIRTransform.startOptimizedFHIRTransformation();
                    console.log('✅ Transformation completed via service');
                } else {
                    console.error('❌ Transformation service not available');
                    alert('Transformation service not initialized. Please refresh the page.');
                }

                if (button) {
                    button.textContent = '✅ Mappings Generated';
                    button.disabled = false;
                }
            } catch (error) {
                console.error('❌ Error generating FHIR mappings:', error);
                alert(`Failed to generate FHIR mappings: ${error.message}`);

                if (button) {
                    button.textContent = '🎯 Auto-Generate Mappings';
                    button.disabled = false;
                }
            }
        } else {
            console.warn('⚠️ No HL7 data available for transformation');
            alert('Please complete Step 2 (HL7 Parsing) first before generating mappings');
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

window.viewRawFHIRJSON = function() {
    console.log('📄 Viewing raw FHIR JSON...');
    const data = window.wizardController?.getCurrentData();

    if (!data?.fhirTransformResult) {
        alert('No FHIR transformation result available. Please complete the transformation first.');
        return;
    }

    const modal = document.createElement('div');
    modal.style.cssText = 'position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.8); z-index: 10000; display: flex; align-items: center; justify-content: center; padding: 20px;';

    const resources = data.fhirTransformResult.fhirResources || [];
    const bundle = data.fhirTransformResult.bundle || null;

    modal.innerHTML = `
        <div style="background: white; border-radius: 12px; padding: 24px; max-width: 1200px; width: 100%; max-height: 90vh; display: flex; flex-direction: column;">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px;">
                <h3 style="margin: 0; color: #1e3a8a;">📄 FHIR Resources (Raw JSON)</h3>
                <div style="display: flex; gap: 8px;">
                    <button onclick="window.copyFHIRJSON()" style="padding: 8px 16px; background: #6366f1; color: white; border: none; border-radius: 6px; cursor: pointer; font-size: 13px;">
                        📋 Copy All
                    </button>
                    <button onclick="window.downloadFHIRJSON()" style="padding: 8px 16px; background: #10b981; color: white; border: none; border-radius: 6px; cursor: pointer; font-size: 13px;">
                        💾 Download
                    </button>
                    <button onclick="window.closeFHIRJSONViewer(this)" style="padding: 8px 16px; background: #f3f4f6; color: #374151; border: none; border-radius: 6px; cursor: pointer; font-size: 13px;">
                        ✖ Close
                    </button>
                </div>
            </div>

            <div style="border: 2px solid #e5e7eb; border-radius: 8px; overflow: hidden; flex: 1; display: flex; flex-direction: column;">
                <!-- Tab Navigation -->
                <div style="display: flex; background: #f9fafb; border-bottom: 2px solid #e5e7eb;">
                    ${resources.map((r, i) => `
                        <button class="fhir-tab" data-index="${i}" onclick="window.switchFHIRTab(${i})"
                                style="padding: 12px 20px; border: none; background: ${i === 0 ? '#1e3a8a' : 'transparent'}; color: ${i === 0 ? 'white' : '#6b7280'}; cursor: pointer; font-weight: 500; font-size: 14px;">
                            ${r.resourceType || 'Resource'} ${i + 1}
                        </button>
                    `).join('')}
                    ${bundle ? `
                        <button class="fhir-tab" data-index="bundle" onclick="window.switchFHIRTab('bundle')"
                                style="padding: 12px 20px; border: none; background: transparent; color: #6b7280; cursor: pointer; font-weight: 500; font-size: 14px;">
                            📦 Bundle
                        </button>
                    ` : ''}
                </div>

                <!-- JSON Content -->
                <div style="flex: 1; overflow: auto; background: #1e1e1e; padding: 20px;">
                    ${resources.map((r, i) => `
                        <pre id="fhir-json-${i}" style="margin: 0; color: #d4d4d4; font-family: 'Courier New', monospace; font-size: 13px; line-height: 1.6; display: ${i === 0 ? 'block' : 'none'};">${window.syntaxHighlightJSON(r)}</pre>
                    `).join('')}
                    ${bundle ? `
                        <pre id="fhir-json-bundle" style="margin: 0; color: #d4d4d4; font-family: 'Courier New', monospace; font-size: 13px; line-height: 1.6; display: none;">${window.syntaxHighlightJSON(bundle)}</pre>
                    ` : ''}
                </div>
            </div>

            <div style="margin-top: 16px; padding: 12px; background: #f0f9ff; border: 1px solid #bfdbfe; border-radius: 6px;">
                <div style="font-size: 13px; color: #1e40af;">
                    <strong>💡 Tip:</strong> This shows the actual FHIR resources generated from your HL7 message.
                    ${resources.length} resource(s) created: ${resources.map(r => r.resourceType).join(', ')}
                </div>
            </div>
        </div>
    `;

    document.body.appendChild(modal);
};

window.switchFHIRTab = function(index) {
    // Hide all tabs
    document.querySelectorAll('[id^="fhir-json-"]').forEach(el => {
        el.style.display = 'none';
    });

    // Update tab styles
    document.querySelectorAll('.fhir-tab').forEach(tab => {
        tab.style.background = 'transparent';
        tab.style.color = '#6b7280';
    });

    // Show selected tab
    const tabId = index === 'bundle' ? 'fhir-json-bundle' : `fhir-json-${index}`;
    const element = document.getElementById(tabId);
    if (element) {
        element.style.display = 'block';
    }

    // Highlight selected tab button
    const button = document.querySelector(`[data-index="${index}"]`);
    if (button) {
        button.style.background = '#1e3a8a';
        button.style.color = 'white';
    }
};

window.copyFHIRJSON = function() {
    const data = window.wizardController?.getCurrentData();
    if (data?.fhirTransformResult) {
        const json = JSON.stringify(data.fhirTransformResult.fhirResources, null, 2);
        navigator.clipboard.writeText(json).then(() => {
            alert('✅ FHIR JSON copied to clipboard!');
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

window.syntaxHighlightJSON = function(obj) {
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