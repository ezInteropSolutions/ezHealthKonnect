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
                // Automatically start FHIR transformation when entering step 3
                await this.autoStartFHIRTransformation();
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

        // Don't overwrite existing name/description with empty values
        // This prevents losing data when syncing from steps that don't have these fields
        if (data.name === '' && this.model.data.name) {
            console.log('⚠️ Skipping empty name update, keeping existing:', this.model.data.name);
            delete data.name;
        }
        if (data.description === '' && this.model.data.description) {
            console.log('⚠️ Skipping empty description update, keeping existing:', this.model.data.description);
            delete data.description;
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

                // The model already updates itself in parseHL7Message,
                // so we just need to refresh the view
                console.log('✅ Parsed HL7 data updated in model:', parseResult.data?.messageType);

                // Show success notification
                this.view.showNotification('HL7 message parsed successfully!', 'success');

                // Re-render the step to show parsed data
                await this.loadStep(2);

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

                // Emit completion event
                if (interfaceId) {
                    this.dispatchEvent(new CustomEvent('wizardCompleted', {
                        detail: {
                            interfaceId: interfaceId,
                            name: payload.name,
                            interface: interfaceData
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
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
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

        // Check if we have parsed HL7 data
        const parsedHL7Data = this.model.data.parsedHL7Data;
        if (!parsedHL7Data) {
            console.warn('⚠️ No parsed HL7 data available for transformation');
            return;
        }

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

// Auto-initialize if container exists (OOB behavior)
document.addEventListener('DOMContentLoaded', () => {
    const container = document.getElementById('wizard-modal-container');
    if (container && !window.wizardController) {
        console.log('🚀 Auto-initializing Optimized Wizard (OOB)');
        window.wizardController = new WizardController();
    }
});