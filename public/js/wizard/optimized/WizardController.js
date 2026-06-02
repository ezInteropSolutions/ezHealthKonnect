/**
 * WizardController.js - Controller for Optimized Interface Wizard
 * Implements MVC Controller pattern following our architecture
 *
 * Features:
 * - Orchestrates Model and View interactions
 * - Handles business logic and API communication
 * - Implements our dual-language architecture (Node.js + Go)
 * - Follows ezHealthKonnect architectural patterns
 * - Integrates with existing backend services
 */

class WizardController extends EventTarget {
    constructor(containerId = 'wizard-modal-container') {
        super();

        // Initialize MVC components
        this.model = new WizardModel();
        this.view = new WizardView(containerId);

        // Set global reference for onclick handlers in the beautiful HL7 element table
        window.wizardView = this.view;
        window.wizardController = this;

        // API configuration following our architecture
        this.apiConfig = {
            nodeBackend: window.location.origin,       // Node.js frontend/proxy
            goBackend: 'http://localhost:8080',        // Go backend (if direct access needed)
            timeout: 30000,
            retryAttempts: 3
        };

        // State management
        this.isInitialized = false;
        this.isProcessing = false;
        this.isLoadingStep = false;
        this.currentOperation = null;

        // Initialize the controller
        this.initialize();
    }

    /**
     * Initialize the wizard controller
     */
    async initialize() {
        try {
            console.log('🎯 Initializing Optimized Wizard Controller...');

            // Bind events between Model and View
            this.bindModelEvents();
            this.bindViewEvents();

            // Don't auto-load step 1 to prevent conflicts
            // Step will be loaded when wizard is opened

            this.isInitialized = true;
            console.log('✅ Wizard Controller initialized successfully');

        } catch (error) {
            console.error('❌ Failed to initialize Wizard Controller:', error);
            this.handleError('Initialization failed', error);
        }
    }

    /**
     * Bind Model events to Controller actions
     */
    bindModelEvents() {
        // Data changes trigger view updates
        this.model.addEventListener('dataChange', (event) => {
            const { step, data, validation } = event.detail;

            // IMPORTANT: Don't re-render Step 1 or Step 4 on every data change
            // Step 1: Prevents input fields from losing focus on every keystroke
            // Step 4: Preserves UI state (shown/hidden sections) when checkboxes/radios change
            if (step === 1 || step === 4) {
                console.log(`ℹ️ Step ${step} data changed, updating validation only (not re-rendering)`);
                this.view.showValidation(validation);
                return;
            }

            // For other steps, re-render as needed
            this.view.renderStep(step, { ...data, validation });
            this.view.showValidation(validation);
        });

        // Step changes trigger navigation updates
        this.model.addEventListener('stepChange', (event) => {
            const { step, validation } = event.detail;
            this.loadStep(step);
        });

        // Template application triggers data reload
        this.model.addEventListener('templateApplied', (event) => {
            const { templateId, template } = event.detail;
            this.view.showNotification(`Applied template: ${template.name}`, 'success');
            this.loadStep(this.model.currentStep);
        });

        // Data import triggers full refresh
        this.model.addEventListener('dataImported', () => {
            this.loadStep(this.model.currentStep);
            this.view.showNotification('Configuration imported successfully', 'success');
        });
    }

    /**
     * Bind View events to Controller actions
     */
    bindViewEvents() {
        console.log('🔗 Binding view events to controller...');
        console.log('🔍 View instance:', this.view);

        // Test event listener to verify binding works
        this.view.addEventListener('test', () => {
            console.log('🧪 Test event received by controller');
        });

        // Test the event system immediately
        setTimeout(() => {
            console.log('🧪 Testing event dispatch...');
            this.view.dispatchEvent(new CustomEvent('test'));
        }, 100);

        // Navigation events
        this.view.addEventListener('nextStep', () => {
            console.log('🎯 Controller received nextStep event');
            this.nextStep();
        });
        this.view.addEventListener('previousStep', () => {
            console.log('🎯 Controller received previousStep event');
            this.previousStep();
        });
        this.view.addEventListener('goToStep', (event) => {
            console.log('🎯 Controller received goToStep event:', event.detail.step);
            this.goToStep(event.detail.step);
        });
        this.view.addEventListener('finish', () => {
            console.log('🎯 Controller received finish event');
            this.finishWizard();
        });
        this.view.addEventListener('close', () => {
            console.log('🎯 Controller received close event');
            this.closeModal();
        });

        // Data input events
        this.view.addEventListener('fieldChange', (event) => {
            const { field, value } = event.detail;
            this.updateField(field, value);
        });

        // Data sync events
        this.view.addEventListener('dataSync', (event) => {
            console.log('🔄 Controller received dataSync event:', event.detail);
            this.updateModelData(event.detail);
        });

        // Template selection
        this.view.addEventListener('templateSelected', (event) => {
            const { template, templateId } = event.detail;
            if (templateId) {
                this.applyTemplate(templateId);
            } else if (template) {
                this.applyOOBTemplate(template);
            }
        });

        // HL7 Parsing events
        this.view.addEventListener('parseHL7Requested', (event) => {
            console.log('🔍 Controller received parseHL7Requested event:', event.detail);
            this.parseHL7Message(event.detail.message, event.detail.escapeHandling);
        });

        this.view.addEventListener('sampleHL7Requested', (event) => {
            console.log('📋 Controller received sampleHL7Requested event:', event.detail);
            this.loadSampleHL7(event.detail.messageType);
        });

        // Testing and validation
        this.view.addEventListener('testConnection', (event) => {
            this.testConnection(event.detail.type);
        });

        this.view.addEventListener('validateMappings', () => {
            this.validateMappings();
        });

        this.view.addEventListener('testTransform', () => {
            this.testTransformation();
        });

        this.view.addEventListener('finalTest', () => {
            this.runFinalTest();
        });

        // Configuration management
        this.view.addEventListener('exportConfiguration', () => {
            this.exportConfiguration();
        });
    }

    /**
     * Load and display a specific step
     */
    async loadStep(stepNumber) {
        try {
            // Prevent recursive calls
            if (this.isLoadingStep) {
                console.log('⏸️ Already loading a step, skipping...');
                return true;
            }

            this.isLoadingStep = true;
            console.log(`📍 Loading step ${stepNumber}...`);

            // Update model's current step (silently, without events)
            this.model.currentStep = stepNumber;
            this.model.validateCurrentStep();

            // Get step data from model
            console.log('📊 Getting step data from model...');
            const stepData = this.model.getCurrentStepData();
            console.log('📋 Step data received:', stepData);

            const validation = this.model.validation;
            console.log('✅ Validation state:', validation);

            // Render step in view
            console.log('🎨 Rendering step', stepNumber, 'with data:', stepData);
            this.view.renderStep(stepNumber, { ...stepData, validation });

            // Update step indicators and navigation
            console.log('🎯 About to update step indicator for step:', stepNumber);
            this.view.updateStepIndicator(stepNumber);
            console.log('🧭 About to update navigation for step:', stepNumber, 'validation:', validation);
            this.view.updateNavigation(stepNumber, validation);

            // Load step-specific data if needed
            await this.loadStepSpecificData(stepNumber);

            this.isLoadingStep = false;
            return true;

        } catch (error) {
            this.isLoadingStep = false;
            console.error(`❌ Failed to load step ${stepNumber}:`, error);
            this.handleError(`Failed to load step ${stepNumber}`, error);
            return false;
        }
    }

    /**
     * Load step-specific data from backend
     */
    async loadStepSpecificData(stepNumber) {
        switch (stepNumber) {
            case 3:
                // Lazy-load mappings for the first message type after DOM renders
                setTimeout(() => {
                    const families = this.model.data.acceptedMessageFamilies;
                    const FAMILY_REPR = { ADT: 'ADT^A01', ORU: 'ORU^R01', ORM: 'ORM^O01', SIU: 'SIU^S12', MDM: 'MDM^T02', MFN: 'MFN^M02', VXU: 'VXU^V04', DFT: 'DFT^P03', BAR: 'BAR^P01', RDE: 'RDE^O11' };
                    let initialType = this.model.data.detectedMessageType || 'ADT^A01';
                    if (families && families.length > 0) {
                        const first = families[0];
                        initialType = first.includes('^') ? first : (FAMILY_REPR[first] || 'ADT^A01');
                    }
                    const firstBtn = document.querySelector(`.msg-type-preview-btn[data-msg-type="${initialType}"]`);
                    window._previewMappingForType(initialType, firstBtn).then(() => {
                        // Pre-warm mapping cache for other common types (silent — no DOM effect)
                        const allCommon = ['ADT^A01', 'ORU^R01', 'ORM^O01', 'OML^O21', 'ADT^A03', 'SIU^S12', 'MDM^T02', 'MFN^M02', 'VXU^V04', 'DFT^P03', 'BAR^P01'];
                        const others = allCommon.filter(t => t !== initialType);
                        window._prewarmMappingCache(others);
                    });
                }, 150);
                break;
            case 4:
                // Ensure Step 4 has access to FHIR transformation result from Step 3
                console.log('🔍 Step 4: Checking for FHIR transformation result...');

                // If no transformation result exists, try to get it from Step 3
                if (!this.model.data.fhirTransformResult) {
                    console.log('⚠️ No FHIR transform result found, checking Step 3 data...');

                    // Check if we have parsed HL7 data to re-run transformation
                    if (this.model.data.parsedHL7Data) {
                        console.log('🔄 Re-running FHIR transformation for Step 4...');
                        await this.autoStartFHIRTransformation();
                    }
                } else {
                    console.log('✅ FHIR transform result available for Step 4:', {
                        success: this.model.data.fhirTransformResult.success,
                        atomicMappings: this.model.data.fhirTransformResult.atomicMappings?.length || 0,
                        validationErrors: this.model.data.fhirTransformResult.validationErrors?.length || 0
                    });
                }

                // Load available OOB templates and mapping configurations
                await this.loadMappingTemplates();
                break;
            case 5:
                // Validate final configuration
                await this.validateFinalConfiguration();

                // Populate deployment settings panel
                const deploymentContainer = document.getElementById('wizardDeploymentSettingsContainer');
                if (deploymentContainer && window.InterfaceConfigComponents) {
                    deploymentContainer.innerHTML = InterfaceConfigComponents.getDeploymentSettingsPanel(
                        this.model.data || {},
                        '' // Wizard uses empty prefix
                    );
                    // Initialize event handlers for deployment settings
                    InterfaceConfigComponents.initDeploymentSettingsEvents(document, '');
                    console.log('✅ Deployment settings panel populated for Step 5');
                }
                break;
        }
    }

    /**
     * Update a field in the model
     */
    updateField(field, value) {
        try {
            const updateData = { [field]: value };

            // Handle nested configuration updates
            if (field.startsWith('source')) {
                updateData.sourceConfig = {
                    ...this.model.data.sourceConfig,
                    [field.replace('source', '').toLowerCase()]: value
                };
            } else if (field.startsWith('target')) {
                updateData.targetConfig = {
                    ...this.model.data.targetConfig,
                    [field.replace('target', '').toLowerCase()]: value
                };
            }

            // Sync FHIR version from transformation flow selection
            if (field === 'transformationFlow') {
                const flowVersionMap = {
                    'hl7_to_fhir':    'R4',
                    'hl7_to_fhir_r5': 'R5',
                    'ccd_to_fhir':    'R4'
                };
                if (flowVersionMap[value]) {
                    updateData.targetConfig = {
                        ...this.model.data.targetConfig,
                        version: flowVersionMap[value]
                    };
                }
            }

            // When FHIR version changes, clear the mapping cache so stale
            // R4 results aren't served for R5 (or vice-versa)
            if (field === 'targetVersion' || field === 'transformationFlow') {
                if (window._mappingPreviewCache) window._mappingPreviewCache.clear();
            }

            this.model.updateStepData(updateData);

        } catch (error) {
            console.error('❌ Failed to update field:', error);
            this.view.showNotification('Failed to update field', 'error');
        }
    }

    /**
     * Apply OOB template based on selection
     */
    applyOOBTemplate(templateType) {
        const templates = {
            'hospital-adt': {
                name: 'Hospital ADT Interface',
                description: 'Standard hospital patient admission/discharge interface',
                sourceType: 'hl7v2',
                sourceConnectivity: 'tcp',
                targetType: 'fhir',
                targetConnectivity: 'http',
                messageType: 'ADT^A01',
                mappingTemplate: 'adt_a01_basic'
            },
            'lab-results': {
                name: 'Laboratory Results Interface',
                description: 'Laboratory reporting and results interface',
                sourceType: 'hl7v2',
                sourceConnectivity: 'tcp',
                targetType: 'fhir',
                targetConnectivity: 'http',
                messageType: 'ORU^R01',
                mappingTemplate: 'oru_r01_basic'
            },
            'radiology': {
                name: 'Radiology Interface',
                description: 'Radiology imaging and reports interface',
                sourceType: 'hl7v2',
                sourceConnectivity: 'tcp',
                targetType: 'fhir',
                targetConnectivity: 'http',
                messageType: 'ORU^R01',
                mappingTemplate: 'oru_r01_basic'
            },
            'custom': {
                name: 'Custom Interface',
                description: 'Custom healthcare integration interface'
                // Keep current settings for custom
            }
        };

        const template = templates[templateType];
        if (template) {
            // Apply template data to model
            this.model.updateStepData(template);

            // Apply mapping template if specified
            if (template.mappingTemplate) {
                this.model.applyTemplate(template.mappingTemplate);
            }

            this.view.showNotification(`Applied ${template.name} template`, 'success');
        }
    }

    /**
     * Update model data from view
     */
    updateModelData(data) {
        console.log('📊 Updating model data:', data);

        // Don't overwrite existing values with empty/undefined — steps that don't
        // render certain fields will return undefined, which must not clobber the model.
        if (data.name === '' && this.model.data.name) {
            console.log('⚠️ Skipping empty name update, keeping existing:', this.model.data.name);
            delete data.name;
        }
        if (data.description === '' && this.model.data.description) {
            console.log('⚠️ Skipping empty description update, keeping existing:', this.model.data.description);
            delete data.description;
        }
        if (data.transformationFlow == null && this.model.data.transformationFlow) {
            delete data.transformationFlow;
        }

        // Update the model's data
        Object.assign(this.model.data, data);

        // Update timestamp
        this.model.data.lastModified = new Date().toISOString();

        console.log('✅ Model data updated - Name:', this.model.data.name, 'Description:', this.model.data.description);
    }

    /**
     * Apply mapping template
     */
    applyTemplate(templateId) {
        if (this.model.applyTemplate(templateId)) {
            this.view.showNotification('Mapping template applied successfully', 'success');
        } else {
            this.view.showNotification('Failed to apply template', 'error');
        }
    }

    /**
     * Modal control methods
     */
    async openModal() {
        try {
            console.log('🚀 Opening optimized wizard modal...');

            // Show the modal
            this.view.openModal();

            // Add a small delay to ensure modal is fully rendered
            setTimeout(async () => {
                console.log('⏰ Delayed step load after modal open');

                // Debug: Check if step container exists
                const stepContainer = document.getElementById('wizardStepContainer');
                console.log('🔍 Step container found:', !!stepContainer);
                if (stepContainer) {
                    console.log('📦 Step container HTML:', stepContainer.outerHTML);
                }

                // Load step 1 when opening (not during initialization)
                console.log('🎯 About to call loadStep(1)...');
                try {
                    const result = await this.loadStep(1);
                    console.log('✅ loadStep(1) completed with result:', result);
                } catch (stepError) {
                    console.error('❌ Error in loadStep(1):', stepError);
                }
            }, 100);

        } catch (error) {
            console.error('❌ Failed to open wizard modal:', error);
            this.view.showNotification('Failed to open wizard', 'error');
        }
    }

    closeModal() {
        try {
            console.log('🚪 Closing optimized wizard modal...');
            this.view.closeModal();

            // Reset to step 1 for next time
            this.model.currentStep = 1;

        } catch (error) {
            console.error('❌ Failed to close wizard modal:', error);
        }
    }

    /**
     * Navigation methods
     */
    async nextStep() {
        console.log('➡️ Controller nextStep called');
        console.log('📊 Model current step:', this.model.currentStep);
        console.log('🔍 Model validation state:', this.model.validation);

        // CRITICAL: Save current step data before HTML is replaced
        console.log('💾 Saving current step data before navigation...');
        const syncedData = this.view.syncFormDataToModel();
        // Update model directly AND synchronously to prevent data loss
        if (syncedData && Object.keys(syncedData).length > 0) {
            this.updateModelData(syncedData);
        }

        // Check for duplicate name before leaving Step 1
        if (this.model.currentStep === 1) {
            const interfaceName = this.view.container.querySelector('#interfaceName')?.value?.trim();
            if (interfaceName && interfaceName.length >= 3) {
                console.log('🔍 Checking duplicate name before leaving Step 1...');
                await this.view.checkDuplicateName(interfaceName);

                // If duplicate was detected, don't proceed
                if (this.view.isDuplicateName) {
                    console.log('⚠️ Cannot proceed: duplicate name detected');
                    this.view.showNotification('Please choose a different interface name', 'warning');
                    return;
                }
            }
        }

        if (this.model.nextStep()) {
            console.log('✅ Model allows next step, loading step:', this.model.currentStep);
            await this.loadStep(this.model.currentStep);
        } else {
            console.log('❌ Model validation failed, showing warning');

            // Show specific validation errors instead of generic message
            const errors = this.model.validation.errors;
            if (errors && Object.keys(errors).length > 0) {
                const errorMessages = Object.entries(errors)
                    .map(([field, message]) => `• ${message}`)
                    .join('\n');
                console.log('❌ Validation errors:', errorMessages);
                this.view.showNotification(
                    `Please fix the following issues:\n\n${errorMessages}`,
                    'warning'
                );
            } else {
                this.view.showNotification('Please resolve validation errors before proceeding', 'warning');
            }
        }
    }

    async previousStep() {
        // CRITICAL: Save current step data before HTML is replaced
        console.log('💾 Saving current step data before navigation...');
        const syncedData = this.view.syncFormDataToModel();
        if (syncedData && Object.keys(syncedData).length > 0) {
            this.updateModelData(syncedData);
        }

        if (this.model.previousStep()) {
            await this.loadStep(this.model.currentStep);
        }
    }

    async goToStep(stepNumber) {
        // CRITICAL: Save current step data before HTML is replaced
        console.log('💾 Saving current step data before navigation...');
        const syncedData = this.view.syncFormDataToModel();
        if (syncedData && Object.keys(syncedData).length > 0) {
            this.updateModelData(syncedData);
        }

        if (this.model.goToStep(stepNumber)) {
            await this.loadStep(stepNumber);
        } else {
            this.view.showNotification('Cannot navigate to that step yet', 'warning');
        }
    }

    /**
     * Test connection based on type (source/target)
     */
    async testConnection(type) {
        try {
            this.view.showLoading(`Testing ${type} connection...`);

            const config = type === 'source' ? this.model.data.sourceConfig : this.model.data.targetConfig;
            const connectivity = type === 'source' ? this.model.data.sourceConnectivity : this.model.data.targetConnectivity;

            let testResult;

            if (connectivity === 'tcp') {
                testResult = await this.testTCPConnection(config);
            } else if (connectivity === 'http') {
                testResult = await this.testHTTPConnection(config);
            } else {
                throw new Error(`Connection testing not supported for ${connectivity}`);
            }

            this.view.hideLoading();

            if (testResult.success) {
                this.view.showNotification(`${type} connection successful`, 'success');
            } else {
                this.view.showNotification(`${type} connection failed: ${testResult.error}`, 'error');
            }

        } catch (error) {
            this.view.hideLoading();
            console.error(`❌ Connection test failed:`, error);
            this.view.showNotification(`Connection test failed: ${error.message}`, 'error');
        }
    }

    /**
     * Test TCP connection
     */
    async testTCPConnection(config) {
        const response = await this.apiCall('/api/wizard/test-tcp-connection', {
            method: 'POST',
            body: JSON.stringify({
                host: config.host,
                port: config.port,
                timeout: config.timeout || 5000
            })
        });

        return response;
    }

    /**
     * Test HTTP connection
     */
    async testHTTPConnection(config) {
        const response = await this.apiCall('/api/wizard/test-http-connection', {
            method: 'POST',
            body: JSON.stringify({
                endpoint: config.endpoint,
                method: 'GET',
                timeout: config.timeout || 5000
            })
        });

        return response;
    }

    /**
     * Load mapping templates from backend
     */
    async loadMappingTemplates() {
        try {
            const response = await this.apiCall('/api/wizard/mapping-templates');

            if (response.success && response.templates) {
                // Update model with available templates
                this.model.oobTemplates = { ...this.model.oobTemplates, ...response.templates };
            }

        } catch (error) {
            console.warn('Failed to load mapping templates:', error);
            // Continue with built-in templates
        }
    }

    /**
     * Validate mappings
     */
    async validateMappings() {
        try {
            this.view.showLoading('Validating mappings...');

            const response = await this.apiCall('/api/wizard/validate-mappings', {
                method: 'POST',
                body: JSON.stringify({
                    messageType: this.model.data.messageType,
                    mappings: this.model.data.customMappings,
                    template: this.model.data.mappingTemplate
                })
            });

            this.view.hideLoading();

            if (response.success) {
                this.view.showNotification('Mappings validated successfully', 'success');
            } else {
                this.view.showNotification(`Validation failed: ${response.error}`, 'error');
            }

        } catch (error) {
            this.view.hideLoading();
            console.error('❌ Mapping validation failed:', error);
            this.view.showNotification('Mapping validation failed', 'error');
        }
    }

    /**
     * Test transformation with sample data
     */
    async testTransformation() {
        try {
            this.view.showLoading('Testing transformation...');

            // Get sample HL7 message for testing
            const sampleMessage = await this.getSampleHL7Message(this.model.data.messageType);

            const response = await this.apiCall('/api/fhir/test-transform-v3', {
                method: 'POST',
                body: JSON.stringify({
                    hl7Message: sampleMessage,
                    mappings: this.model.data.customMappings,
                    messageType: this.model.data.messageType,
                    fhirVersion: this.model.data.targetConfig.version || 'R4'
                })
            });

            this.view.hideLoading();

            if (response.success) {
                this.view.showNotification('Transformation test successful', 'success');

                // Show preview in UI if available
                this.displayTransformationPreview(response.fhirBundle);
            } else {
                this.view.showNotification(`Transformation failed: ${response.error}`, 'error');
            }

        } catch (error) {
            this.view.hideLoading();
            console.error('❌ Transformation test failed:', error);
            this.view.showNotification('Transformation test failed', 'error');
        }
    }

    /**
     * Load a sample HL7 message into the textarea
     */
    loadSampleHL7(messageType) {
        try {
            this.model.useSampleHL7Message(messageType);
            const sampleMessage = this.model.data.hl7Message;

            // Populate the textarea directly
            const textarea = document.getElementById('hl7Message') ||
                             document.querySelector('.hl7-textarea');
            if (textarea) {
                textarea.value = sampleMessage;
                textarea.dispatchEvent(new Event('input'));
            }

            // Also update model data so the parse button picks it up
            this.model.data.hl7Message = sampleMessage;
            console.log('✅ Sample HL7 loaded:', messageType);

            // Auto-parse so results show immediately
            this.parseHL7Message(sampleMessage, false);
        } catch (err) {
            console.error('❌ loadSampleHL7 error:', err);
        }
    }

    /**
     * Parse HL7 message
     */
    async parseHL7Message(hl7Content, escapeHandling) {
        try {
            console.log('🔍 Controller: Starting HL7 message parsing...');
            this.view.showLoading('Parsing HL7 message...');

            // Get the HL7 content from current step data if not provided
            if (!hl7Content) {
                const stepData = this.model.getCurrentStepData();
                hl7Content = stepData.hl7Message;
            }

            if (!hl7Content) {
                throw new Error('No HL7 message content to parse');
            }

            // Call the model's parseHL7Message method which handles the API call
            const parseResult = await this.model.parseHL7Message(hl7Content, escapeHandling);

            if (parseResult.success) {
                console.log('✅ HL7 parsing successful:', parseResult);

                // ── Batch detection ──────────────────────────────────────────
                if (parseResult.isBatch && parseResult.messageCount > 1) {
                    console.log(`📦 Batch detected: ${parseResult.messageCount} messages`);
                    this._showBatchBanner(parseResult.messageCount, parseResult.batchMessages);
                }

                // The model already updates itself in parseHL7Message,
                // so we just need to refresh the view
                console.log('✅ Parsed HL7 data updated in model:', parseResult.data?.messageType);

                // Show success notification
                const msgLabel = parseResult.isBatch
                    ? `${parseResult.messageCount} HL7 messages detected and parsed`
                    : 'HL7 message parsed successfully!';
                this.view.showNotification(msgLabel, 'success');

                // Re-render the step to show parsed data
                await this.loadStep(2);

                // ── Z-segment banner ─────────────────────────────────────────
                const zSegs = parseResult.detectedZSegments || this.model.data.detectedZSegments || [];
                const zData = parseResult.detectedZSegmentData || this.model.data.detectedZSegmentData || [];
                if (zSegs.length > 0) {
                    console.log(`🔍 ${zSegs.length} Z-segment(s) detected — showing banner`);
                    this._showZSegmentBanner(zSegs, zData);
                }

            } else {
                throw new Error(parseResult.error || 'HL7 parsing failed');
            }

        } catch (error) {
            console.error('❌ HL7 parsing failed:', error);
            this.view.showNotification(`HL7 parsing failed: ${error.message}`, 'error');
        } finally {
            this.view.hideLoading();
        }
    }

    // _showBatchBanner injects a persistent info banner into the Step 2 container
    // listing all detected messages and their types.
    _showBatchBanner(count, entries) {
        // Remove any stale banner from a previous parse
        document.querySelectorAll('.hl7-batch-banner').forEach(el => el.remove());

        const rows = (entries || []).map((e, i) => {
            const mt = e.data?.messageType?.name || e.data?.messageType?.code || '—';
            const status = e.success
                ? `<span style="color:#16a34a;">✓ OK</span>`
                : `<span style="color:#dc2626;">✗ ${e.error || 'parse error'}</span>`;
            return `<tr>
                <td style="padding:3px 8px;border:1px solid #bbf7d0;">${i + 1}</td>
                <td style="padding:3px 8px;border:1px solid #bbf7d0;font-family:monospace;">${mt}</td>
                <td style="padding:3px 8px;border:1px solid #bbf7d0;">${status}</td>
            </tr>`;
        }).join('');

        const banner = document.createElement('div');
        banner.className = 'hl7-batch-banner';
        banner.style.cssText = `
            background:#f0fdf4; border:2px solid #86efac; border-radius:10px;
            padding:14px 18px; margin-bottom:16px; font-size:13px;
        `;
        banner.innerHTML = `
            <div style="display:flex;align-items:center;gap:10px;margin-bottom:10px;">
                <span style="font-size:20px;">📦</span>
                <div>
                    <strong style="color:#15803d;font-size:14px;">${count} HL7 messages detected in this input</strong><br>
                    <span style="color:#166534;">The wizard uses the <strong>first message</strong> for mapping configuration.
                    To transform all messages use the <strong>Batch Transform</strong> feature.</span>
                </div>
            </div>
            <table style="border-collapse:collapse;width:100%;">
                <thead><tr style="background:#dcfce7;">
                    <th style="padding:4px 8px;border:1px solid #bbf7d0;text-align:left;">#</th>
                    <th style="padding:4px 8px;border:1px solid #bbf7d0;text-align:left;">Message Type</th>
                    <th style="padding:4px 8px;border:1px solid #bbf7d0;text-align:left;">Status</th>
                </tr></thead>
                <tbody>${rows}</tbody>
            </table>
        `;

        const step2 = document.getElementById('step2') || document.querySelector('.step-content');
        if (step2) step2.insertBefore(banner, step2.firstChild);
    }

    /**
     * Show a Z-segment detection banner below the parsing results in Step 2.
     * Lists each detected Z-segment, whether an OOB template is available,
     * and informs the user that templates will be auto-applied on finish.
     */
    _showZSegmentBanner(detectedSegments, segmentData) {
        // Remove any stale Z-seg banner from a previous parse
        document.querySelectorAll('.zseg-wizard-banner').forEach(el => el.remove());

        const esc = s => String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

        const rows = detectedSegments.map(segId => {
            const detail = segmentData.find(d => d.segmentId === segId) || {};
            const hasOOB = detail.hasOobTemplate || false;
            const oobTmpl = (detail.suggestedTemplates || []).find(t => t.isSystem);
            const badge = hasOOB
                ? `<span style="background:#166534;color:#fff;padding:1px 7px;border-radius:10px;font-size:11px;">OOB Template</span>`
                : `<span style="background:#6b7280;color:#fff;padding:1px 7px;border-radius:10px;font-size:11px;">Custom</span>`;
            const note = hasOOB
                ? `Will auto-apply <em>${esc(oobTmpl?.templateName || 'OOB template')}</em> → ${esc(oobTmpl?.targetFhirResource || '')}`
                : `No OOB template — configure manually in Interface Settings after creation`;
            return `<tr>
                <td style="padding:4px 10px;border:1px solid #a5f3fc;font-family:monospace;font-weight:700;">
                    ${esc(segId)}</td>
                <td style="padding:4px 10px;border:1px solid #a5f3fc;">${badge}</td>
                <td style="padding:4px 10px;border:1px solid #a5f3fc;font-size:12px;color:#164e63;">${note}</td>
            </tr>`;
        }).join('');

        const oobCount = segmentData.filter(d => d.hasOobTemplate).length;
        const autoNote = oobCount > 0
            ? `<strong>${oobCount} OOB template${oobCount > 1 ? 's' : ''} will be applied automatically</strong> when you finish the wizard.`
            : `No OOB templates available — configure Z-segments manually after the interface is created.`;

        const banner = document.createElement('div');
        banner.className = 'zseg-wizard-banner';
        banner.style.cssText = `
            background:#ecfeff; border:2px solid #67e8f9; border-radius:10px;
            padding:14px 18px; margin-bottom:16px; font-size:13px;
        `;
        banner.innerHTML = `
            <div style="display:flex;align-items:center;gap:10px;margin-bottom:10px;">
                <span style="font-size:20px;">🔧</span>
                <div>
                    <strong style="color:#0e7490;font-size:14px;">
                        ${detectedSegments.length} Z-segment${detectedSegments.length > 1 ? 's' : ''} detected</strong><br>
                    <span style="color:#164e63;">${autoNote}</span>
                </div>
            </div>
            <table style="border-collapse:collapse;width:100%;">
                <thead><tr style="background:#cffafe;">
                    <th style="padding:4px 10px;border:1px solid #a5f3fc;text-align:left;">Segment</th>
                    <th style="padding:4px 10px;border:1px solid #a5f3fc;text-align:left;">Template</th>
                    <th style="padding:4px 10px;border:1px solid #a5f3fc;text-align:left;">Action</th>
                </tr></thead>
                <tbody>${rows}</tbody>
            </table>
        `;

        // Insert after parsingResults so it appears below the parsed segment table
        const parsingResults = document.getElementById('parsingResults');
        const step2 = document.getElementById('step2') || document.querySelector('.step-content');
        if (parsingResults && parsingResults.parentNode) {
            parsingResults.parentNode.insertBefore(banner, parsingResults.nextSibling);
        } else if (step2) {
            step2.appendChild(banner);
        }
    }

    /**
     * Auto-apply OOB Z-segment templates to the newly created interface.
     * Called right after finishWizard() obtains the interfaceId.
     * Returns an array of segment IDs that were successfully applied.
     */
    /**
     * Save Z-segment configurations for the newly created interface.
     * Uses the ZSegmentConfigPanel's getConfigs() result rather than blindly
     * applying the OOB template — the user may have customized the field mappings
     * or defined a mapping for an unknown segment.
     *
     * Each entry from getConfigs() is one of:
     *   { mode: 'apply-template', segmentId, templateId }  — OOB accepted as-is
     *   { mode: 'create-config',  segmentId, config }      — user-defined / customized
     *   { mode: 'skip',           segmentId }              — user left unconfigured
     *
     * Returns an array of segment IDs that were successfully saved.
     */
    async _autoApplyZSegmentTemplates(interfaceId) {
        // Prefer the panel's configs when available (Step 3 was rendered)
        const panel = this._zSegmentConfigPanel;
        const configs = panel ? panel.getConfigs() : this._buildFallbackConfigs();

        if (configs.length === 0) return [];

        const applied = [];
        for (const entry of configs) {
            if (entry.mode === 'skip') {
                console.log(`⏭️ Z-segment ${entry.segmentId} skipped (not configured)`);
                continue;
            }

            try {
                let res, json;

                if (entry.mode === 'apply-template') {
                    res = await fetch(`/api/zsegments/interfaces/${interfaceId}/apply-template`, {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ templateId: entry.templateId, occurrenceIndex: 0 }),
                    });
                } else {
                    // 'create-config' — user defined or customized
                    res = await fetch(`/api/zsegments/interfaces/${interfaceId}`, {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify(entry.config),
                    });
                }

                json = await res.json();
                if (json.success) {
                    console.log(`✅ Z-segment ${entry.segmentId} saved (mode: ${entry.mode})`);
                    applied.push(entry.segmentId);
                } else {
                    console.warn(`⚠️ Failed to save Z-segment ${entry.segmentId}:`, json.error);
                }
            } catch (err) {
                console.warn(`⚠️ Error saving Z-segment ${entry.segmentId}:`, err.message);
            }
        }
        return applied;
    }

    /**
     * Fallback for when the wizard is completed without rendering Step 3
     * (e.g. the user skipped directly to finish).  Falls back to the old
     * behaviour of applying any available OOB template as-is.
     */
    _buildFallbackConfigs() {
        return (this.model.data.detectedZSegmentData || [])
            .filter(seg => seg.hasOobTemplate)
            .map(seg => {
                const oob = (seg.suggestedTemplates || []).find(t => t.isSystem);
                return oob
                    ? { mode: 'apply-template', segmentId: seg.segmentId, templateId: oob.id }
                    : { mode: 'skip', segmentId: seg.segmentId };
            });
    }

    /**
     * Get sample HL7 message for testing
     */
    async getSampleHL7Message(messageType) {
        const sampleMessages = {
            'ADT^A01': `MSH|^~\\&|EPIC|HOSPITAL|FHIR|DESTINATION|20230101120000||ADT^A01|123456|P|2.5
PID|1||12345^^^MRN||Doe^John^M||19800101|M|||123 Main St^^Anytown^ST^12345
PV1|1|I|ICU^101^1|||DOC123^Smith^Jane|||EMERGENCY|||`,

            'ORU^R01': `MSH|^~\\&|LAB|HOSPITAL|FHIR|DESTINATION|20230101120000||ORU^R01|123456|P|2.5
PID|1||12345^^^MRN||Doe^John^M||19800101|M|||123 Main St^^Anytown^ST^12345
OBR|1||12345|CBC^Complete Blood Count|||20230101120000
OBX|1|NM|WBC^White Blood Cell Count|4500|cells/uL|4000-11000|N|||`,

            'ADT^A03': `MSH|^~\\&|EPIC|HOSPITAL|FHIR|DESTINATION|20230101120000||ADT^A03|123456|P|2.5
PID|1||12345^^^MRN||Doe^John^M||19800101|M|||123 Main St^^Anytown^ST^12345
PV1|1|I|ICU^101^1|||DOC123^Smith^Jane|||EMERGENCY|||`
        };

        return sampleMessages[messageType] || sampleMessages['ADT^A01'];
    }

    /**
     * Display transformation preview in UI
     */
    displayTransformationPreview(fhirBundle) {
        const previewContainer = document.getElementById('fhirPreview');
        if (previewContainer && fhirBundle) {
            previewContainer.innerHTML = `
                <pre class="json-preview">${JSON.stringify(fhirBundle, null, 2)}</pre>
            `;
        }
    }

    /**
     * Validate final configuration
     */
    async validateFinalConfiguration() {
        try {
            // Perform comprehensive validation
            const validationResult = await this.apiCall('/api/wizard/validate-final-config', {
                method: 'POST',
                body: JSON.stringify(this.model.getApiPayload())
            });

            if (!validationResult.success) {
                this.model.validation.errors.summary = validationResult.error;
                this.model.validation.isValid = false;
            }

        } catch (error) {
            console.warn('Final validation failed:', error);
            // Continue - validation will be done server-side
        }
    }

    /**
     * Run final comprehensive test
     */
    async runFinalTest() {
        try {
            this.view.showLoading('Running comprehensive test...');

            const response = await this.apiCall('/api/wizard/final-test', {
                method: 'POST',
                body: JSON.stringify(this.model.getApiPayload())
            });

            this.view.hideLoading();

            if (response.success) {
                this.view.showNotification('All tests passed! Ready to create interface.', 'success');
            } else {
                this.view.showNotification(`Tests failed: ${response.error}`, 'error');
            }

        } catch (error) {
            this.view.hideLoading();
            console.error('❌ Final test failed:', error);
            this.view.showNotification('Final test failed', 'error');
        }
    }

    /**
     * Export configuration for backup/sharing
     */
    exportConfiguration() {
        try {
            const config = this.model.exportData();
            const blob = new Blob([JSON.stringify(config, null, 2)], {
                type: 'application/json'
            });

            const url = URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = `${config.name || 'interface'}-config.json`;
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            URL.revokeObjectURL(url);

            this.view.showNotification('Configuration exported successfully', 'success');

        } catch (error) {
            console.error('❌ Export failed:', error);
            this.view.showNotification('Export failed', 'error');
        }
    }

    /**
     * Finish wizard and create interface
     */
    async finishWizard() {
        if (this.isProcessing) {
            console.log('⏳ Already processing wizard completion, ignoring duplicate request');
            return;
        }

        // Set processing flag immediately to prevent race conditions
        this.isProcessing = true;

        try {
            this.currentOperation = 'creating_interface';

            console.log('🎯 Starting wizard completion process...');

            // CRITICAL: Ensure all visible form data is synced before finishing
            console.log('💾 Final sync of all form data...');
            const syncedData = this.view.syncFormDataToModel();
            if (syncedData && Object.keys(syncedData).length > 0) {
                this.updateModelData(syncedData);
            }

            this.view.showLoading('Creating interface...');

            // Final validation
            if (!this.model.validateCurrentStep()) {
                throw new Error('Configuration validation failed');
            }

            // Prepare API payload following our architecture
            const payload = this.model.getApiPayload();

            console.log('📤 Sending interface creation request:', {
                name: payload.name,
                sourceType: payload.sourceType,
                targetType: payload.targetType,
                mappingsCount: payload.mappings?.length || 0
            });

            // Call our backend API (Node.js proxy → Go backend)
            const response = await this.apiCall('/api/wizard/complete', {
                method: 'POST',
                body: JSON.stringify({ wizardData: payload })
            });

            this.view.hideLoading();

            if (response.success) {
                // Handle both response formats: {data: {interfaceId}} or {interfaceId}
                const interfaceId = response.data?.interfaceId || response.interfaceId;
                const interfaceData = response.data || response.interface || response;

                console.log('✅ Interface created successfully:', interfaceData);

                // ── Auto-apply Z-segment OOB templates ───────────────────────
                // If the sample message had Z-segments with OOB templates, apply
                // them now that we have the interfaceId.
                let appliedZSegments = [];
                if (interfaceId) {
                    appliedZSegments = await this._autoApplyZSegmentTemplates(interfaceId);
                }

                // Get the interface status to show appropriate message
                const status = interfaceData.status || 'draft';
                const messageType = interfaceData.message_type || payload.messageType || 'ADT^A01';

                // Show completion modal with action buttons instead of simple notification
                this.view.showCompletionModal({
                    interfaceId: interfaceId,
                    name: payload.name,
                    status: status,
                    messageType: messageType,
                    autoStart: payload.auto_start || false
                });

                // If Z-segment templates were applied, show a follow-up notification
                if (appliedZSegments.length > 0) {
                    setTimeout(() => {
                        this.view.showNotification(
                            `Z-segment mappings applied: ${appliedZSegments.join(', ')}. ` +
                            `Edit them in Interface Settings → Z-Segments.`,
                            'success',
                            8000
                        );
                    }, 800);
                }

                // Emit completion event
                if (interfaceId) {
                    this.dispatchEvent(new CustomEvent('wizardCompleted', {
                        detail: {
                            interfaceId: interfaceId,
                            name: payload.name,
                            interface: interfaceData,
                            appliedZSegments
                        }
                    }));
                } else {
                    console.warn('⚠️ Backend returned success but no interface ID. Response:', response);
                }

                // Refresh interfaces list in background
                if (typeof window.loadInterfaces === 'function') {
                    window.loadInterfaces();
                }

            } else {
                throw new Error(response.error || 'Interface creation failed');
            }

        } catch (error) {
            this.view.hideLoading();
            console.error('❌ Wizard completion failed:', error);

            this.view.showNotification(
                `Failed to create interface: ${error.message}`,
                'error',
                5000
            );

            this.handleError('Interface creation failed', error);

        } finally {
            this.isProcessing = false;
            this.currentOperation = null;
        }
    }

    /**
     * Close wizard with cleanup
     */
    closeWizard() {
        try {
            console.log('🚪 Closing wizard...');

            // Hide the view
            this.view.hide();

            // Reset model after successful completion (no confirmation needed)
            // If wizard was completed via finishWizard(), data is already saved
            this.model.reset();

            // Emit close event
            this.dispatchEvent(new CustomEvent('wizardClosed'));

        } catch (error) {
            console.error('❌ Error closing wizard:', error);
        }
    }

    /**
     * Show wizard modal
     */
    show() {
        this.view.show();
        this.loadStep(1);

        // Emit show event
        this.dispatchEvent(new CustomEvent('wizardShown'));
    }

    /**
     * Hide wizard modal
     */
    hide() {
        this.view.hide();

        // Emit hide event
        this.dispatchEvent(new CustomEvent('wizardHidden'));
    }

    /**
     * Import configuration from file/data
     */
    importConfiguration(configData) {
        try {
            this.model.importData(configData);
            this.loadStep(this.model.currentStep);

        } catch (error) {
            console.error('❌ Import failed:', error);
            this.view.showNotification('Failed to import configuration', 'error');
        }
    }

    /**
     * API call helper following our architecture
     */
    async apiCall(endpoint, options = {}) {
        const defaultOptions = {
            method: 'GET',
            headers: {
                'Content-Type': 'application/json',
                ...options.headers
            },
            credentials: 'same-origin'  // Include session cookies
        };

        const finalOptions = { ...defaultOptions, ...options };

        // Add timeout
        const controller = new AbortController();
        const timeoutId = setTimeout(() => controller.abort(), this.apiConfig.timeout);
        finalOptions.signal = controller.signal;

        try {
            const url = `${this.apiConfig.nodeBackend}${endpoint}`;
            console.log(`📡 API Call: ${finalOptions.method} ${url}`);

            const response = await fetch(url, finalOptions);
            clearTimeout(timeoutId);

            if (!response.ok) {
                let errorMsg = `HTTP ${response.status}: ${response.statusText}`;
                try {
                    const errBody = await response.json();
                    if (errBody?.error) errorMsg = errBody.error;
                } catch (_) {}
                throw new Error(errorMsg);
            }

            const data = await response.json();

            console.log(`✅ API Response: ${endpoint}`, {
                success: data.success,
                hasData: !!data.data
            });

            return data;

        } catch (error) {
            clearTimeout(timeoutId);

            if (error.name === 'AbortError') {
                throw new Error('Request timeout');
            }

            console.error(`❌ API Call failed: ${endpoint}`, error);
            throw error;
        }
    }

    /**
     * Error handling following our patterns
     */
    handleError(context, error) {
        console.error(`❌ ${context}:`, error);

        // Log error for debugging
        const errorDetails = {
            context,
            message: error.message,
            stack: error.stack,
            timestamp: new Date().toISOString(),
            wizardStep: this.model.currentStep,
            operation: this.currentOperation
        };

        // Store error for potential reporting
        this.lastError = errorDetails;

        // Show user-friendly message
        this.view.showNotification(
            `${context}. Please try again or contact support.`,
            'error',
            5000
        );
    }

    /**
     * Get wizard status for debugging
     */
    getStatus() {
        return {
            isInitialized: this.isInitialized,
            isProcessing: this.isProcessing,
            currentOperation: this.currentOperation,
            currentStep: this.model.currentStep,
            isValid: this.model.validation.isValid,
            lastError: this.lastError,
            timestamp: new Date().toISOString()
        };
    }

    /**
     * Reset wizard to initial state
     */
    reset() {
        this.model.reset();
        this.loadStep(1);
        this.view.showNotification('Wizard reset to initial state', 'info');
    }

    /**
     * Check for duplicate interface name
     */
    async checkDuplicateName(name) {
        try {
            const response = await this.apiCall('/api/wizard/check-duplicate', {
                method: 'POST',
                body: JSON.stringify({ name })
            });

            return response.isDuplicate || false;

        } catch (error) {
            console.warn('Duplicate check failed:', error);
            return false; // Assume not duplicate on error
        }
    }

    /**
     * Automatically start FHIR transformation when entering step 3
     */
    async autoStartFHIRTransformation() {
        console.log('🔄 Auto-starting FHIR transformation...');

        // If no parsed HL7 data (user skipped Step 2), auto-load a representative sample
        if (!this.model.data.parsedHL7Data) {
            console.log('ℹ️ No parsed HL7 data — auto-loading sample for mapping preview...');

            const families = this.model.data.acceptedMessageFamilies;
            const FAMILY_REPR = { ADT: 'ADT^A01', ORU: 'ORU^R01', ORM: 'ORM^O01', SIU: 'SIU^S12', MDM: 'MDM^T02', MFN: 'MFN^M02', VXU: 'VXU^V04', DFT: 'DFT^P03', BAR: 'BAR^P01', RDE: 'RDE^O11' };
            let sampleType = 'ADT^A01';
            if (families && families.length > 0) {
                const first = families[0];
                sampleType = first.includes('^') ? first : (FAMILY_REPR[first] || 'ADT^A01');
            }

            await new Promise((resolve) => {
                let resolved = false;
                const done = () => { if (!resolved) { resolved = true; resolve(); } };
                this.model.addEventListener('hl7Parsed', done, { once: true });
                setTimeout(done, 5000); // safety timeout
                this.loadSampleHL7(sampleType);
            });

            if (!this.model.data.parsedHL7Data) {
                console.warn('⚠️ Auto-load sample failed — skipping FHIR transformation');
                return;
            }
            console.log('✅ Sample loaded:', sampleType);
        }

        const parsedHL7Data = this.model.data.parsedHL7Data;
        console.log('📊 Parsed HL7 data available:', parsedHL7Data);

        // Use the optimized FHIR transform service
        if (window.optimizedFHIRTransform) {
            try {
                await window.optimizedFHIRTransform.startOptimizedFHIRTransformation();
                console.log('✅ Automatic FHIR transformation completed');
            } catch (error) {
                console.error('❌ Automatic FHIR transformation failed:', error);
            }
        } else {
            console.error('❌ FHIR transform service not available');
        }
    }

    /**
     * Get current wizard data (used by helper functions in WizardView)
     */
    getCurrentData() {
        return this.model.data;
    }

    /**
     * Update wizard data with new values
     */
    updateData(newData) {
        this.model.updateStepData(newData);
    }

    /**
     * Re-render the current step
     */
    renderCurrentStep() {
        if (this.view) {
            this.view.render(this.model.data);
        }
    }
}

// Make available globally following our patterns
window.WizardController = WizardController;

/**
 * Skip the HL7 parsing step (Step 2) without validation.
 * Step 2 is optional; FHIR transform will auto-load a sample if needed.
 */
window._skipWizardHL7Step = function() {
    const ctrl = window.wizardController;
    if (!ctrl) return;
    ctrl.model.currentStep = 3;
    ctrl.loadStep(3);
};

// Per-session result cache keyed by "messageType:fhirVersion:preset"
window._mappingPreviewCache = window._mappingPreviewCache || new Map();

// Parse-only cache keyed by messageType — eliminates first network call on re-visits
window._parsedSampleCache = window._parsedSampleCache || new Map();

/** Refresh the coverage summary bar after a mapping preview loads. */
function _updateCoverageBar(atomicMappings, validationErrors) {
    const bar = document.getElementById('mapping-coverage-bar');
    if (!bar) return;
    const total  = atomicMappings.length;
    const high   = atomicMappings.filter(m => m.confidence == null || m.confidence >= 0.90).length;
    const med    = atomicMappings.filter(m => m.confidence != null && m.confidence >= 0.75 && m.confidence < 0.90).length;
    const low    = atomicMappings.filter(m => m.confidence != null && m.confidence < 0.75).length;
    const errors = (validationErrors || []).length;
    bar.innerHTML = `
        <span style="font-weight:600;">📊 ${total} fields mapped</span>
        <span style="color:#d1d5db;">|</span>
        ${high > 0 ? `<span style="color:#16a34a;">● ${high} high</span>` : ''}
        ${med  > 0 ? `<span style="color:#d97706;">● ${med} medium</span>` : ''}
        ${low  > 0 ? `<span style="color:#dc2626;">● ${low} low</span>` : ''}
        <span style="color:#d1d5db;">|</span>
        <span style="${errors > 0 ? 'color:#dc2626;font-weight:600;' : 'color:#16a34a;'}">
            ${errors > 0 ? `⚠ ${errors} validation issue${errors > 1 ? 's' : ''}` : '✓ No validation issues'}
        </span>`;
}

/** Repopulate the resource-filter dropdown from a set of atomic mappings. */
function _updateResourceFilter(atomicMappings) {
    const filterSelect = document.getElementById('resource-filter');
    if (!filterSelect) return;
    const names = [...new Set(
        atomicMappings.map(m => {
            const path = m.resourceType || m.targetPath || m.fhirPath || m.fhirField || '';
            const match = path.match(/^([A-Z][A-Za-z]+)/);
            return match ? match[1] : null;
        }).filter(Boolean)
    )];
    filterSelect.innerHTML = '<option value="all">All Resources</option>'
        + names.map(r => `<option value="${r}">${r}</option>`).join('');
}
// AbortController for the currently in-flight fetch pair
window._mappingPreviewAbort = null;

/**
 * Skeleton shimmer HTML shown while fetch is in flight.
 * Mirrors the visual weight of renderFHIRMappingEditor so the layout
 * doesn't jump when the real content arrives.
 */
function _mappingSkeletonHTML(messageType) {
    const rows = Array(8).fill(0).map(() => `
        <div style="border:1px solid #e2e8f0;border-radius:8px;padding:12px;margin-bottom:8px;display:flex;gap:12px;align-items:center;">
            <div class="skshimmer" style="width:110px;height:13px;border-radius:4px;flex-shrink:0;"></div>
            <div class="skshimmer" style="width:20px;height:13px;border-radius:4px;flex-shrink:0;"></div>
            <div class="skshimmer" style="width:170px;height:13px;border-radius:4px;"></div>
            <div class="skshimmer" style="width:55px;height:20px;border-radius:4px;margin-left:auto;flex-shrink:0;"></div>
        </div>`).join('');
    return `
        <style>
            .skshimmer{background:linear-gradient(90deg,#f0f0f0 25%,#e0e0e0 50%,#f0f0f0 75%);background-size:200% 100%;animation:skwave 1.4s infinite linear;}
            @keyframes skwave{0%{background-position:200% 0}100%{background-position:-200% 0}}
        </style>
        <div style="padding:16px;">
            <div style="background:#f0fdf4;border:2px solid #bbf7d0;border-radius:12px;padding:14px 16px;margin-bottom:16px;display:flex;align-items:center;gap:12px;">
                <div class="skshimmer" style="width:28px;height:28px;border-radius:50%;flex-shrink:0;"></div>
                <div style="flex:1;">
                    <div class="skshimmer" style="width:220px;height:15px;border-radius:4px;margin-bottom:6px;"></div>
                    <div class="skshimmer" style="width:310px;height:12px;border-radius:4px;"></div>
                </div>
            </div>
            <div style="display:flex;gap:8px;margin-bottom:14px;">
                <div class="skshimmer" style="width:175px;height:32px;border-radius:8px;"></div>
                <div class="skshimmer" style="width:110px;height:32px;border-radius:8px;"></div>
                <div class="skshimmer" style="width:90px;height:32px;border-radius:8px;margin-left:auto;"></div>
            </div>
            ${rows}
        </div>`;
}

/**
 * Preview FHIR mappings for a specific HL7 message type without touching model state.
 * Called from the message-type pill selector in Step 3 (getStep4Template).
 */
window._previewMappingForType = async function(messageType, btnEl, options = {}) {
    const ctrl = window.wizardController;
    if (!ctrl) return;
    const silent = options.silent === true; // Pre-warm mode: populate cache without touching the DOM

    const fhirVersion = ctrl.model.data.targetConfig?.version || 'R4';
    const preset = ctrl.model.data.mappingOptions?.transformationPreset || 'standard';
    const cacheKey = `${messageType}:${fhirVersion}:${preset}`;

    // Highlight active pill (skip in silent/pre-warm mode)
    if (!silent) {
        document.querySelectorAll('.msg-type-preview-btn').forEach(b => {
            b.style.background = 'white';
            b.style.color = '#6b7280';
            b.style.borderColor = '#e5e7eb';
        });
        if (btnEl) {
            btnEl.style.background = '#1e3a8a';
            btnEl.style.color = 'white';
            btnEl.style.borderColor = '#1e3a8a';
        }
    }

    const mappingContainer = document.getElementById('fhir-mapping-container');

    // --- Cache hit: render immediately, no network round-trip ---
    if (window._mappingPreviewCache.has(cacheKey)) {
        const cached = window._mappingPreviewCache.get(cacheKey);
        if (mappingContainer && ctrl.view) {
            const previewData = {
                ...ctrl.model.getCurrentStepData(),
                detectedMessageType: cached.effectiveType,
                fhirTransformResult: cached.transformResult,
                previewHL7Version: cached.hl7Version
            };
            mappingContainer.innerHTML = ctrl.view.getFHIRMappingContent(previewData);
            const countEl = document.getElementById('mapping-count');
            if (countEl) countEl.textContent = cached.transformResult.atomicMappings.length;
            const typeEl = document.getElementById('fhir-message-type');
            if (typeEl) typeEl.textContent = cached.effectiveType + (cached.effectiveType !== messageType ? ' (fallback)' : '');
            const versionEl = document.getElementById('fhir-hl7-version');
            if (versionEl) versionEl.textContent = 'HL7 v' + cached.hl7Version;
            _updateResourceFilter(cached.transformResult.atomicMappings);
            _updateCoverageBar(cached.transformResult.atomicMappings, cached.transformResult.validationErrors);
        }
        if (ctrl.model?.data) {
            ctrl.model.data.fhirTransformResult = cached.transformResult;
            ctrl.model.data.detectedMessageType = ctrl.model.data.detectedMessageType || cached.effectiveType;
        }
        ctrl.view?.validateCurrentStep?.();
        return;
    }

    // --- Cache miss: show skeleton and fetch ---
    const loadingEl = document.getElementById('msg-type-preview-loading');
    let signal;
    if (silent) {
        // Pre-warm: use an independent controller so we never abort the visible request
        signal = new AbortController().signal;
    } else {
        // Visible load: cancel any previous in-flight visible request and show skeleton
        if (loadingEl) loadingEl.style.display = 'inline';
        if (mappingContainer) {
            mappingContainer.innerHTML = _mappingSkeletonHTML(messageType);
        }
        if (window._mappingPreviewAbort) {
            window._mappingPreviewAbort.abort();
        }
        window._mappingPreviewAbort = new AbortController();
        signal = window._mappingPreviewAbort.signal;
    }

    // Complete, validated sample HL7 messages per type (hoisted to window for pre-parse access)
    if (!window._SAMPLE_HL7) window._SAMPLE_HL7 = {
        'ADT^A01': [
            'MSH|^~\\&|EPIC|HOSPITAL|FHIR|DEST|20230101120000||ADT^A01|MSG001|P|2.5',
            'EVN|A01|20230101120000',
            'PID|1||12345^^^MRN^MR||Doe^John^M||19800101|M|||123 Main St^^Anytown^ST^12345||555-1234|||||S||123-45-6789',
            'PV1|1|I|ICU^101^1^HOSPITAL|||001234^Smith^Jane^A^^^MD|||SUR||||A|||001234^Smith^Jane^A^^^MD|INP|VN12345||||||||||||||||||HOSPITAL|||||20230101120000'
        ].join('\n'),
        'ADT^A03': [
            'MSH|^~\\&|EPIC|HOSPITAL|FHIR|DEST|20230101120000||ADT^A03|MSG002|P|2.5',
            'EVN|A03|20230101120000',
            'PID|1||12345^^^MRN^MR||Doe^John^M||19800101|M|||123 Main St^^Anytown^ST^12345',
            'PV1|1|I|ICU^101^1^HOSPITAL|||001234^Smith^Jane^A^^^MD|||SUR||||A|||001234^Smith^Jane^A^^^MD|INP|VN12345||||||||||||||||||HOSPITAL|||||20230101120000|20230105143000'
        ].join('\n'),
        'ORU^R01': [
            'MSH|^~\\&|LAB|HOSPITAL|FHIR|DEST|20230101120000||ORU^R01|MSG003|P|2.5',
            'PID|1||12345^^^MRN^MR||Doe^John^M||19800101|M|||123 Main St^^Anytown^ST^12345',
            'OBR|1|ORD001|LAB001|85025^CBC with Differential^LN|||20230101110000|||||||||001234^Smith^Jane^^^MD|||||||||F',
            'OBX|1|NM|6690-2^WBC^LN||7.5|10*3/uL|4.5-11.0|N|||F|||20230101120000',
            'OBX|2|NM|789-8^RBC^LN||4.8|10*6/uL|4.2-5.4|N|||F|||20230101120000',
            'OBX|3|NM|718-7^HGB^LN||14.5|g/dL|12.0-16.0|N|||F|||20230101120000'
        ].join('\n'),
        'ORM^O01': [
            'MSH|^~\\&|ORDER|HOSPITAL|FHIR|DEST|20230101120000||ORM^O01|MSG004|P|2.5',
            'PID|1||12345^^^MRN^MR||Doe^John^M||19800101|M|||123 Main St^^Anytown^ST^12345',
            'ORC|NW|ORD001^ORDER||||||20230101120000|||001234^Smith^Jane^^^MD',
            'OBR|1|ORD001^ORDER||85025^CBC with Differential^LN|||20230101120000|||||||||001234^Smith^Jane^^^MD'
        ].join('\n'),
        'SIU^S12': [
            'MSH|^~\\&|SCH|HOSPITAL|FHIR|DEST|20230101120000||SIU^S12|MSG005|P|2.5',
            'SCH|APT001||||||ROUTINE^Routine|60|min^^ISO+|^^^20230201090000^^R|||001234^Smith^Jane^^^MD|OFFICE^Office^L',
            'PID|1||12345^^^MRN^MR||Doe^John^M||19800101|M|||123 Main St^^Anytown^ST^12345',
            'AIG|1||001234^Smith^Jane^^^MD^||||20230201090000|60|min^^ISO+'
        ].join('\n'),
        'MDM^T02': [
            'MSH|^~\\&|HIM|HOSPITAL|FHIR|DEST|20230101120000||MDM^T02|MSG006|P|2.5',
            'EVN|T02|20230101120000',
            'PID|1||12345^^^MRN^MR||Doe^John^M||19800101|M|||123 Main St^^Anytown^ST^12345',
            'TXA|1|DS|TX|20230101120000|001234^Smith^Jane^^^MD||20230101120000||AP|||DOC001||001234^Smith^Jane^^^MD||AU',
            'OBX|1|TX|34133-9^Summary of episode note^LN||Patient presented with acute chest pain and was admitted for observation. Discharged in stable condition after 48 hours.'
        ].join('\n'),
        'MFN^M02': [
            'MSH|^~\\&|MFN|HOSPITAL|FHIR|DEST|20230101120000||MFN^M02|MSG007|P|2.5',
            'MFI|STF^Staff Master File^HL7||UPD|||AL',
            'MFE|MAD|20230101120000|20230101120000|STAFF001^Staff001^L',
            'STF|STAFF001^Smith^Jane^A||Smith^Jane^A^MD|P|F|19700601|||MD^Physician^HL7|HOSPITAL^^^LOC|123-456-7890|||A'
        ].join('\n'),
        'VXU^V04': [
            'MSH|^~\\&|EHR|CLINIC|FHIR|DEST|20230101120000||VXU^V04|MSG008|P|2.5',
            'PID|1||12345^^^MRN^MR||Doe^John^M||19800101|M|||123 Main St^^Anytown^ST^12345',
            'ORC|RE|IMM001||||||20230101120000|||001234^Smith^Jane^^^MD',
            'RXA|0|1|20230101|20230101|141^Influenza, seasonal, injectable^CVX||1|mL^milliliter^UCUM|01^Historical^NIP001|||||||UNK^Unknown manufacturer^MVX||CP|A'
        ].join('\n'),
        'DFT^P03': [
            'MSH|^~\\&|BILLING|HOSPITAL|FHIR|DEST|20230101120000||DFT^P03|MSG009|P|2.5',
            'EVN|P03|20230101120000',
            'PID|1||12345^^^MRN^MR||Doe^John^M||19800101|M|||123 Main St^^Anytown^ST^12345',
            'PV1|1|I|ICU^101^1^HOSPITAL|||001234^Smith^Jane^^^MD|||SUR||||A',
            'FT1|1|||20230101|20230101|CG|99213^Office visit, est, low complexity^CPT4||1|250.00||USD|||001234^Smith^Jane^^^MD'
        ].join('\n'),
        'BAR^P01': [
            'MSH|^~\\&|ADMIT|HOSPITAL|FHIR|DEST|20230101120000||BAR^P01|MSG010|P|2.5',
            'EVN|P01|20230101120000',
            'PID|1||12345^^^MRN^MR||Doe^John^M||19800101|M|||123 Main St^^Anytown^ST^12345',
            'PV1|1|I|ICU^101^1^HOSPITAL|||001234^Smith^Jane^^^MD|||SUR||||A|||001234^Smith^Jane^^^MD|INP|VN12345||||||||||||||||||HOSPITAL|||||20230101120000'
        ].join('\n'),
        'OML^O21': [
            'MSH|^~\\&|LIS|HOSPITAL|FHIR|DEST|20230101120000||OML^O21|MSG011|P|2.5',
            'PID|1||12345^^^MRN^MR||Doe^John^M||19800101|M|||123 Main St^^Anytown^ST^12345',
            'ORC|NW|ORD001^ORDER||||||20230101120000|||001234^Smith^Jane^^^MD',
            'OBR|1|ORD001^ORDER||85025^CBC with Differential^LN|||20230101120000|||||||||001234^Smith^Jane^^^MD',
            'SPM|1|||BLDV^Blood venous^HL70487|||||||P||||||20230101120000|20230101120000'
        ].join('\n'),
    };
    const SAMPLE_HL7 = window._SAMPLE_HL7;

    const attemptParse = async (hl7Msg, cacheKey) => {
        if (cacheKey && window._parsedSampleCache.has(cacheKey)) {
            return window._parsedSampleCache.get(cacheKey);
        }
        const resp = await fetch('/api/hl7/parse', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ rawMessage: hl7Msg.trim(), useEnhanced: true, escapeHandling: 'passthrough' }),
            signal
        });
        if (!resp.ok) throw new Error(`${resp.status}`);
        const json = await resp.json();
        const result = json.data || json;
        if (cacheKey) window._parsedSampleCache.set(cacheKey, result);
        return result;
    };

    try {
        let effectiveType = messageType;
        let catalogueJson;

        // Fast path: single DB read (~50ms) — no HL7 parse, no FHIR bundle, no schema loader
        try {
            const catalogueResp = await fetch(
                `/api/fhir/mapping-catalogue?messageType=${encodeURIComponent(messageType)}&fhirVersion=${encodeURIComponent(fhirVersion)}`,
                { signal }
            );
            if (!catalogueResp.ok) throw new Error(`${catalogueResp.status}`);
            catalogueJson = await catalogueResp.json();
            if (!catalogueJson.atomicMappings?.length) throw new Error('empty catalogue');
        } catch (catalogueErr) {
            if (catalogueErr.name === 'AbortError') throw catalogueErr;
            // Backend fell back to ADT^A01 already — but if the endpoint itself failed, try ADT explicitly
            console.warn(`⚠️ Catalogue fetch failed for ${messageType} (${catalogueErr.message}), falling back to ADT^A01`);
            const fallbackResp = await fetch(
                `/api/fhir/mapping-catalogue?messageType=${encodeURIComponent('ADT^A01')}&fhirVersion=${encodeURIComponent(fhirVersion)}`,
                { signal }
            );
            if (!fallbackResp.ok) throw new Error(`Catalogue fallback failed: ${fallbackResp.status}`);
            catalogueJson = await fallbackResp.json();
            effectiveType = 'ADT^A01';
        }

        const atomicMappings = catalogueJson.atomicMappings || [];
        const hl7Version = catalogueJson.hl7Version || '2.5';
        const transformResult = { ...catalogueJson, atomicMappings };

        // Store in cache so revisiting this pill is instant
        window._mappingPreviewCache.set(cacheKey, { effectiveType, transformResult, hl7Version });

        if (!silent) {
            // Update the visible mapping container only for non-background loads
            if (mappingContainer && ctrl.view) {
                const previewData = {
                    ...ctrl.model.getCurrentStepData(),
                    detectedMessageType: effectiveType,
                    fhirTransformResult: transformResult,
                    previewHL7Version: hl7Version
                };
                mappingContainer.innerHTML = ctrl.view.getFHIRMappingContent(previewData);

                const countEl = document.getElementById('mapping-count');
                if (countEl) countEl.textContent = atomicMappings.length;
                const typeEl = document.getElementById('fhir-message-type');
                if (typeEl) typeEl.textContent = effectiveType + (effectiveType !== messageType ? ' (fallback)' : '');
                const versionEl = document.getElementById('fhir-hl7-version');
                if (versionEl) versionEl.textContent = 'HL7 v' + hl7Version;
                _updateResourceFilter(atomicMappings);
                _updateCoverageBar(atomicMappings, catalogueJson.validationErrors);
            }

            if (ctrl.model?.data) {
                ctrl.model.data.fhirTransformResult = transformResult;
                ctrl.model.data.detectedMessageType = ctrl.model.data.detectedMessageType || effectiveType;
            }

            // Re-enable Next once we have mapping results (covers the step-2-skip scenario)
            ctrl.view?.validateCurrentStep?.();
        }

    } catch (err) {
        if (err.name === 'AbortError') return; // Superseded by a newer pill click — silently discard
        console.error('❌ _previewMappingForType error:', err);
        if (mappingContainer) {
            mappingContainer.innerHTML = `<div style="padding:40px;text-align:center;color:#dc2626;font-size:14px;">Failed to load ${messageType} mapping preview: ${err.message}</div>`;
        }
    } finally {
        if (loadingEl) loadingEl.style.display = 'none';
    }
};

/**
 * Pre-parse all sample HL7 messages in parallel so the first pill click only needs
 * one network call (transform) instead of two (parse + transform).
 * Called as soon as Step 3 DOM is ready — runs entirely in the background.
 */
window._preParseSamples = function() {
    const samples = window._SAMPLE_HL7;
    if (!samples) return; // Not yet defined — will be populated on first pill click
    const allTypes = Object.keys(samples);
    allTypes.forEach(type => {
        if (window._parsedSampleCache.has(type)) return;
        // Fire-and-forget; no AbortSignal — background only, not user-triggered
        fetch('/api/hl7/parse', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ rawMessage: samples[type].trim(), useEnhanced: true, escapeHandling: 'passthrough' })
        }).then(r => r.ok ? r.json() : null)
          .then(json => { if (json) window._parsedSampleCache.set(type, json.data || json); })
          .catch(() => {});
    });
};

/**
 * Silently pre-warms the mapping cache for a list of message types.
 * Called after the first pill loads so that subsequent pill clicks feel instant.
 */
window._prewarmMappingCache = function(types) {
    const ctrl = window.wizardController;
    if (!ctrl) return;
    const fhirVersion = ctrl.model.data.targetConfig?.version || 'R4';
    const preset = ctrl.model.data.mappingOptions?.transformationPreset || 'standard';
    types.forEach((type, i) => {
        const key = `${type}:${fhirVersion}:${preset}`;
        if (!window._mappingPreviewCache.has(key)) {
            // Silent background pre-warm — never touches the DOM, never aborts the visible request
            setTimeout(() => window._previewMappingForType(type, null, { silent: true }), i * 400);
        }
    });
};

/** Persists a change from the Advanced Mapping Options panel into the model. */
window._onAdvancedOptionChange = function(key, value) {
    const ctrl = window.wizardController;
    if (!ctrl) return;
    ctrl.model.data.mappingOptions = { ...ctrl.model.data.mappingOptions, [key]: value };
    // Changing transformationPreset affects mapping output — clear cache and re-run active pill
    if (key === 'transformationPreset') {
        if (window._mappingPreviewCache) window._mappingPreviewCache.clear();
        const activeBtn = document.querySelector('.msg-type-preview-btn.active');
        const activeType = activeBtn?.dataset?.msgType || ctrl.model.data.detectedMessageType || 'ADT^A01';
        window._previewMappingForType(activeType, activeBtn);
    }
};

/**
 * Called when the user changes the FHIR version selector inline on the mapping step.
 * Updates the model, clears stale cache entries, then re-runs the active pill.
 */
window._onMappingFHIRVersionChange = function(newVersion) {
    const ctrl = window.wizardController;
    if (!ctrl) return;

    // Update model
    ctrl.model.data.targetConfig = { ...ctrl.model.data.targetConfig, version: newVersion };

    // Clear entire cache — all entries are for the old version
    if (window._mappingPreviewCache) window._mappingPreviewCache.clear();

    // Re-run the currently active pill
    const activeBtn = document.querySelector('.msg-type-preview-btn[style*="background: rgb(30, 58, 138)"], .msg-type-preview-btn[style*="background:#1e3a8a"], .msg-type-preview-btn.active');
    const activeType = activeBtn?.dataset?.msgType
        || ctrl.model.data.detectedMessageType
        || 'ADT^A01';
    window._previewMappingForType(activeType, activeBtn);
};

// Auto-initialize if container exists (OOB behavior)
document.addEventListener('DOMContentLoaded', () => {
    const container = document.getElementById('wizard-modal-container');
    if (container && !window.wizardController) {
        console.log('🚀 Auto-initializing Optimized Wizard (OOB)');
        window.wizardController = new WizardController();
    }
});