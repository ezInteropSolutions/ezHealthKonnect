/**
 * WizardModel.js - Data Model for Optimized Interface Wizard
 * Implements MVC Model pattern with OOB (Out-of-Box) defaults
 *
 * Features:
 * - Centralized state management
 * - Built-in validation rules
 * - OOB templates and defaults
 * - Event-driven updates
 */

class WizardModel extends EventTarget {
    constructor() {
        super();
        this.reset();
        this.initializeOOBTemplates();
    }

    /**
     * Reset wizard to initial state with OOB defaults
     */
    reset() {
        this.data = {
            // Step 1: Interface Basic Info
            name: '',
            description: '',

            // Step 2: Source Configuration (OOB defaults)
            sourceType: 'hl7v2',           // OOB: Most common
            sourceConnectivity: 'tcp',      // OOB: Standard HL7 transport
            sourceConfig: {
                host: 'localhost',          // OOB: Local development
                port: 2575,                 // OOB: Standard HL7 port
                encoding: 'utf8',           // OOB: Standard encoding
                timeout: 30000              // OOB: 30 second timeout
            },

            // Step 3: Target Configuration (OOB defaults)
            targetType: 'fhir',            // OOB: FHIR R4 target
            targetConnectivity: 'http',     // OOB: REST API
            targetConfig: {
                endpoint: 'http://localhost:8080/fhir',  // OOB: Local FHIR server
                version: 'R4',             // OOB: FHIR R4 standard
                format: 'json',            // OOB: JSON format
                timeout: 30000             // OOB: 30 second timeout
            },

            // Step 4: HL7 Parsing & Sample
            hl7Message: '',                // User provided or sample HL7
            useSampleHL7: false,          // Whether to use built-in sample
            parsedHL7Data: null,          // Parsed HL7 message structure
            detectedMessageType: '',      // Auto-detected message type

            // Step 5: Mapping Configuration (OOB templates)
            messageType: 'ADT^A01',        // OOB: Most common message
            mappingTemplate: 'adt_a01_basic', // OOB: Built-in template
            customMappings: [],            // User customizations

            // Step 6: Summary and Completion
            status: 'draft',               // OOB: Start as draft

            // Metadata
            createdAt: new Date().toISOString(),
            lastModified: new Date().toISOString(),
            version: '1.0'
        };

        this.validation = {
            errors: {},
            warnings: {},
            isValid: true
        };

        this.currentStep = 1;
        this.totalSteps = 4;
    }

    /**
     * Initialize OOB templates for common use cases
     */
    initializeOOBTemplates() {
        this.oobTemplates = {
            // Standard ADT A01 (Admit Patient)
            adt_a01_basic: {
                name: 'ADT^A01 - Patient Admission',
                messageType: 'ADT^A01',
                description: 'Standard patient admission message mapping',
                mappings: [
                    { hl7Path: 'MSH.3', fhirPath: 'MessageHeader.source.name', required: true },
                    { hl7Path: 'MSH.4', fhirPath: 'MessageHeader.source.endpoint', required: true },
                    { hl7Path: 'PID.3', fhirPath: 'Patient.identifier[0].value', required: true },
                    { hl7Path: 'PID.5.1', fhirPath: 'Patient.name[0].family', required: true },
                    { hl7Path: 'PID.5.2', fhirPath: 'Patient.name[0].given[0]', required: true },
                    { hl7Path: 'PID.7', fhirPath: 'Patient.birthDate', required: false },
                    { hl7Path: 'PID.8', fhirPath: 'Patient.gender', required: false },
                    { hl7Path: 'PV1.2', fhirPath: 'Encounter.class.code', required: true },
                    { hl7Path: 'PV1.3', fhirPath: 'Encounter.location[0].location.reference', required: false }
                ]
            },

            // Lab Results (ORU R01)
            oru_r01_basic: {
                name: 'ORU^R01 - Lab Results',
                messageType: 'ORU^R01',
                description: 'Standard lab results message mapping',
                mappings: [
                    { hl7Path: 'MSH.3', fhirPath: 'MessageHeader.source.name', required: true },
                    { hl7Path: 'PID.3', fhirPath: 'Patient.identifier[0].value', required: true },
                    { hl7Path: 'PID.5.1', fhirPath: 'Patient.name[0].family', required: true },
                    { hl7Path: 'PID.5.2', fhirPath: 'Patient.name[0].given[0]', required: true },
                    { hl7Path: 'OBR.4.1', fhirPath: 'DiagnosticReport.code.coding[0].code', required: true },
                    { hl7Path: 'OBX.3.1', fhirPath: 'Observation.code.coding[0].code', required: true },
                    { hl7Path: 'OBX.5', fhirPath: 'Observation.valueString', required: true },
                    { hl7Path: 'OBX.6', fhirPath: 'Observation.valueQuantity.unit', required: false }
                ]
            },

            // Discharge (ADT A03)
            adt_a03_basic: {
                name: 'ADT^A03 - Patient Discharge',
                messageType: 'ADT^A03',
                description: 'Standard patient discharge message mapping',
                mappings: [
                    { hl7Path: 'MSH.3', fhirPath: 'MessageHeader.source.name', required: true },
                    { hl7Path: 'PID.3', fhirPath: 'Patient.identifier[0].value', required: true },
                    { hl7Path: 'PID.5.1', fhirPath: 'Patient.name[0].family', required: true },
                    { hl7Path: 'PV1.36', fhirPath: 'Encounter.hospitalization.dischargeDisposition.coding[0].code', required: false },
                    { hl7Path: 'PV1.45', fhirPath: 'Encounter.period.end', required: true }
                ]
            }
        };
    }

    /**
     * Get current step data
     */
    getCurrentStepData() {
        switch (this.currentStep) {
            case 1:
                return {
                    name: this.data.name,
                    description: this.data.description,
                    sourceType: this.data.sourceType,
                    sourceConnectivity: this.data.sourceConnectivity,
                    sourceConfig: this.data.sourceConfig
                };
            case 2:
                return {
                    sourceType: this.data.sourceType,
                    sourceConnectivity: this.data.sourceConnectivity,
                    sourceConfig: this.data.sourceConfig,
                    // Include HL7 parsing data since we consolidated steps
                    hl7Message: this.data.hl7Message,
                    useSampleHL7: this.data.useSampleHL7,
                    parsedHL7Data: this.data.parsedHL7Data,
                    detectedMessageType: this.data.detectedMessageType
                };
            case 3:
                return {
                    hl7Message: this.data.hl7Message,
                    useSampleHL7: this.data.useSampleHL7,
                    parsedHL7Data: this.data.parsedHL7Data,
                    detectedMessageType: this.data.detectedMessageType,
                    // Include FHIR transformation result for mapping display
                    fhirTransformResult: this.data.fhirTransformResult
                };
            case 4:
                return {
                    // Include basic interface info from Step 1
                    name: this.data.name,
                    description: this.data.description,
                    // Include source config from Step 1
                    sourceType: this.data.sourceType,
                    sourceConnectivity: this.data.sourceConnectivity,
                    sourceConfig: this.data.sourceConfig,
                    transformationFlow: this.data.transformationFlow,
                    // Target configuration
                    targetType: this.data.targetType,
                    targetConnectivity: this.data.targetConnectivity,
                    targetConfig: this.data.targetConfig,
                    // Include FHIR transformation result and HL7 data for context
                    fhirTransformResult: this.data.fhirTransformResult,
                    parsedHL7Data: this.data.parsedHL7Data,
                    detectedMessageType: this.data.detectedMessageType
                };
            case 5:
                return {
                    messageType: this.data.messageType,
                    mappingTemplate: this.data.mappingTemplate,
                    customMappings: this.data.customMappings,
                    availableTemplates: this.oobTemplates,
                    parsedHL7Data: this.data.parsedHL7Data
                };
            case 6:
                return this.data;
            default:
                return {};
        }
    }

    /**
     * Update step data with validation
     */
    updateStepData(stepData) {
        const previousData = { ...this.data };

        // Merge new data
        Object.assign(this.data, stepData);
        this.data.lastModified = new Date().toISOString();

        // Validate the changes
        this.validateCurrentStep();

        // Emit change event
        this.dispatchEvent(new CustomEvent('dataChange', {
            detail: {
                step: this.currentStep,
                data: this.data,
                previousData,
                validation: this.validation
            }
        }));

        return this.validation.isValid;
    }

    /**
     * Apply OOB template
     */
    applyTemplate(templateId) {
        const template = this.oobTemplates[templateId];
        if (!template) {
            console.warn(`Template ${templateId} not found`);
            return false;
        }

        // Apply template data
        this.data.messageType = template.messageType;
        this.data.mappingTemplate = templateId;
        this.data.customMappings = [...template.mappings];

        // Emit template applied event
        this.dispatchEvent(new CustomEvent('templateApplied', {
            detail: { templateId, template }
        }));

        return true;
    }

    /**
     * Validate current step
     */
    validateCurrentStep() {
        this.validation.errors = {};
        this.validation.warnings = {};

        switch (this.currentStep) {
            case 1:
                this.validateStep1();
                break;
            case 2:
                this.validateStep2();
                break;
            case 3:
                this.validateStep4(); // FHIR transformation validation
                break;
            case 4:
                this.validateStep3(); // Target config validation
                break;
            case 5:
                this.validateStep5();
                break;
            case 6:
                this.validateStep6();
                break;
        }

        this.validation.isValid = Object.keys(this.validation.errors).length === 0;
        return this.validation.isValid;
    }

    /**
     * Step 1 validation: Basic info
     */
    validateStep1() {
        if (!this.data.name || this.data.name.trim().length < 3) {
            this.validation.errors.name = 'Interface name must be at least 3 characters';
        }

        if (this.data.name && this.data.name.length > 100) {
            this.validation.errors.name = 'Interface name cannot exceed 100 characters';
        }

        // OOB: Auto-generate description if empty
        if (!this.data.description && this.data.name) {
            this.data.description = `HL7 to FHIR interface for ${this.data.name}`;
            this.validation.warnings.description = 'Auto-generated description (OOB)';
        }
    }

    /**
     * Step 2 validation: Source configuration
     */
    validateStep2() {
        if (!this.data.sourceType) {
            this.validation.errors.sourceType = 'Source type is required';
        }

        if (!this.data.sourceConnectivity) {
            this.validation.errors.sourceConnectivity = 'Source connectivity is required';
        }

        // Validate source config based on type
        if (this.data.sourceConnectivity === 'tcp') {
            if (!this.data.sourceConfig.host) {
                this.validation.errors.sourceHost = 'Host is required for TCP connectivity';
            }
            if (!this.data.sourceConfig.port || this.data.sourceConfig.port < 1 || this.data.sourceConfig.port > 65535) {
                this.validation.errors.sourcePort = 'Valid port number (1-65535) is required';
            }
        }
    }

    /**
     * Step 3 validation: Target configuration
     */
    validateStep3() {
        // More permissive validation for step 3 to allow navigation to step 4
        // Target type defaults if not provided
        if (!this.data.targetType) {
            this.data.targetType = 'FHIR'; // Set default
            this.validation.warnings.targetType = 'Default target type set to FHIR';
        }

        if (!this.data.targetConnectivity) {
            this.data.targetConnectivity = 'http'; // Set default
            this.validation.warnings.targetConnectivity = 'Default connectivity set to HTTP';
        }

        // Set default target config if missing
        if (!this.data.targetConfig) {
            this.data.targetConfig = {
                endpoint: 'http://localhost:8081/fhir',
                format: 'json'
            };
            this.validation.warnings.targetConfig = 'Default FHIR endpoint configured';
        }

        // Validate target config only if provided
        if (this.data.targetConnectivity === 'http' && this.data.targetConfig && this.data.targetConfig.endpoint) {
            try {
                new URL(this.data.targetConfig.endpoint);
            } catch {
                this.validation.warnings.targetEndpoint = 'Please verify the target endpoint URL format';
            }
        }
    }

    /**
     * Step 4 validation: FHIR Transformation
     */
    validateStep4() {
        // For FHIR transformation step, we need parsed HL7 data
        if (!this.data.parsedHL7Data) {
            this.validation.warnings.parsedHL7Data = 'HL7 message should be parsed in step 2 before transformation';
            // But don't block navigation - allow user to see FHIR transformation interface
        }

        // Check if we have source HL7 data for transformation
        if (!this.data.hl7Message && !this.data.useSampleHL7) {
            this.validation.warnings.hl7Message = 'HL7 message required for FHIR transformation';
            // But don't block navigation - allow user to see transformation interface
        }

        // Always allow step 4 to be valid so user can see FHIR transformation interface
        // Any required data can be handled by the FHIR transformation component itself
    }

    /**
     * Step 5 validation: Mapping configuration
     */
    validateStep5() {
        if (!this.data.messageType) {
            this.validation.errors.messageType = 'Message type is required';
        }

        if (!this.data.mappingTemplate && this.data.customMappings.length === 0) {
            this.validation.errors.mappings = 'Either select a template or create custom mappings';
        }

        // Validate custom mappings
        this.data.customMappings.forEach((mapping, index) => {
            if (!mapping.hl7Path || !mapping.fhirPath) {
                this.validation.errors[`mapping_${index}`] = 'Both HL7 and FHIR paths are required';
            }
        });
    }

    /**
     * Step 6 validation: Final validation
     */
    validateStep6() {
        // Re-validate all previous steps
        const currentStep = this.currentStep;
        let allValid = true;

        for (let step = 1; step <= 4; step++) {
            this.currentStep = step;
            if (!this.validateCurrentStep()) {
                allValid = false;
            }
        }

        this.currentStep = currentStep;
        this.validation.isValid = allValid;

        if (!allValid) {
            this.validation.errors.summary = 'Please resolve errors in previous steps';
        }
    }

    /**
     * Navigation methods
     */
    canGoToStep(step) {
        console.log('🔍 canGoToStep called for step:', step, 'current:', this.currentStep);

        if (step <= this.currentStep) {
            console.log('✅ Can go to previous/same step');
            return true;
        }

        // FHIR Skip Exception: Allow jumping from step 1 to step 4 for FHIR sources
        if (this.currentStep === 1 && step === 4) {
            const sourceType = this.data.sourceType;
            if (sourceType && sourceType.toLowerCase() === 'fhir') {
                console.log('✅ FHIR skip allowed: step 1 → step 4');
                // Still validate step 1 before allowing skip
                const isValid = this.validateCurrentStep();
                console.log('🔍 Step 1 validation for FHIR skip:', isValid);
                return isValid;
            }
        }

        if (step > this.currentStep + 1) {
            console.log('❌ Cannot skip steps');
            return false;
        }

        // Can only advance if current step is valid
        console.log('🔍 Checking current step validation...');
        const isValid = this.validateCurrentStep();
        console.log('📋 Model validateCurrentStep result:', isValid);
        console.log('📋 Model validation state:', this.validation);
        return isValid;
    }

    goToStep(step) {
        console.log('📋 Model goToStep called:', step);
        const canGo = this.canGoToStep(step);
        console.log('📋 Model canGoToStep result:', canGo);

        if (!canGo) {
            console.log('❌ Model cannot go to step', step);
            return false;
        }

        this.currentStep = step;
        this.validateCurrentStep();

        console.log('✅ Model step changed to:', this.currentStep);

        this.dispatchEvent(new CustomEvent('stepChange', {
            detail: { step: this.currentStep, validation: this.validation }
        }));

        return true;
    }

    nextStep() {
        console.log('📋 Model nextStep called, current:', this.currentStep);

        let nextStepNum = this.currentStep + 1;

        // FHIR Step Skip Logic: Skip HL7-specific steps (2 & 3) for FHIR sources
        if (this.currentStep === 1) {
            const sourceType = this.data.sourceType;
            console.log('🔍 Source type detected:', sourceType);

            // If source is FHIR, skip steps 2 & 3 (HL7 upload and parsing)
            if (sourceType && sourceType.toLowerCase() === 'fhir') {
                console.log('🎯 FHIR source detected - skipping steps 2 & 3 (HL7 upload/parsing)');
                nextStepNum = 4; // Jump directly to mapping configuration
            }
        }

        const result = this.goToStep(nextStepNum);
        console.log('📋 Model nextStep result:', result);
        return result;
    }

    previousStep() {
        let prevStepNum = Math.max(1, this.currentStep - 1);

        // FHIR Step Skip Logic: Skip HL7-specific steps (2 & 3) when going back
        if (this.currentStep === 4) {
            const sourceType = this.data.sourceType;

            // If source is FHIR, skip back to step 1 (bypassing steps 2 & 3)
            if (sourceType && sourceType.toLowerCase() === 'fhir') {
                console.log('🎯 FHIR source detected - skipping back to step 1 (bypassing steps 2 & 3)');
                prevStepNum = 1; // Jump back to interface configuration
            }
        }

        return this.goToStep(prevStepNum);
    }

    /**
     * Get summary for API submission
     */
    getApiPayload() {
        // Extract message type from parsed data or FHIR result
        const messageType = this.data.detectedMessageType ||
                           this.data.parsedHL7Data?.messageType?.name ||
                           this.data.messageType ||
                           'ADT^A01';

        console.log('🔍 getApiPayload - Current model data:', {
            name: this.data.name,
            description: this.data.description,
            sourceType: this.data.sourceType,
            targetType: this.data.targetType,
            mappingsCount: this.data.fhirTransformResult?.atomicMappings?.length
        });

        // Deep log targetConfig to identify JSON issues
        console.log('🔍 targetConfig DETAILED:', JSON.stringify(this.data.targetConfig, null, 2));
        console.log('🔍 targetConfig TYPE:', typeof this.data.targetConfig);
        console.log('🔍 targetConfig KEYS:', this.data.targetConfig ? Object.keys(this.data.targetConfig) : 'null/undefined');

        const payload = {
            // Basic interface info
            name: this.data.name,
            description: this.data.description,

            // Source configuration
            sourceType: this.data.sourceType,
            sourceConnectivity: this.data.sourceConnectivity,
            sourceConfig: this.data.sourceConfig,

            // Target configuration
            targetType: this.data.targetType || 'fhir',
            targetConnectivity: this.data.targetConnectivity || 'http',
            targetConfig: this.data.targetConfig || {},

            // Message type and mappings
            messageType: messageType,

            // Include atomic mappings from FHIR transformation
            mappings: this.data.fhirTransformResult?.atomicMappings || this.data.customMappings || [],

            // Include FHIR transformation result for validation
            fhirTransformResult: this.data.fhirTransformResult,

            // Include parsed HL7 data for enhanced segments
            parsedHL7Data: this.data.parsedHL7Data,

            // Additional metadata
            templateUsed: this.data.mappingTemplate,
            status: this.data.status || 'active'
        };

        // Test JSON stringification of the entire payload
        try {
            const testJSON = JSON.stringify(payload);
            console.log('✅ Payload JSON stringification successful, length:', testJSON.length);
        } catch (err) {
            console.error('❌ PAYLOAD JSON STRINGIFICATION FAILED:', err.message);
            console.error('❌ Problematic payload structure:', payload);
        }

        return payload;
    }

    /**
     * Parse HL7 message
     */
    async parseHL7Message(hl7Content) {
        try {
            console.log('📋 Parsing HL7 message...');

            // Basic validation
            if (!hl7Content || typeof hl7Content !== 'string') {
                throw new Error('Invalid HL7 content provided');
            }

            if (!hl7Content.includes('MSH|') && !hl7Content.startsWith('MSH')) {
                throw new Error('Content does not appear to be a valid HL7 message');
            }

            // Call the Go backend API for parsing
            const response = await fetch('/api/hl7/parse', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({
                    rawMessage: hl7Content.trim(),
                    useEnhanced: true
                })
            });

            if (!response.ok) {
                const errorText = await response.text();
                throw new Error(`HTTP ${response.status}: ${errorText}`);
            }

            const result = await response.json();
            console.log('✅ HL7 message parsed successfully');

            // Update model data
            this.data.hl7Message = hl7Content;
            this.data.parsedHL7Data = result.data || result;
            this.data.detectedMessageType = this.extractMessageType(result.data || result);
            this.data.lastModified = new Date().toISOString();

            // Emit parse completed event
            this.dispatchEvent(new CustomEvent('hl7Parsed', {
                detail: {
                    parsedData: this.data.parsedHL7Data,
                    messageType: this.data.detectedMessageType
                }
            }));

            return {
                success: true,
                data: this.data.parsedHL7Data,
                messageType: this.data.detectedMessageType
            };

        } catch (error) {
            console.error('❌ HL7 parsing failed:', error);
            return {
                success: false,
                error: error.message
            };
        }
    }

    /**
     * Extract message type from parsed HL7 data
     */
    extractMessageType(parsedData) {
        try {
            if (parsedData?.messageType?.name) {
                return parsedData.messageType.name;
            }

            if (parsedData?.segments?.MSH?.fields?.[8]) {
                return parsedData.segments.MSH.fields[8];
            }

            if (parsedData?.MSH?.['9']) {
                return parsedData.MSH['9'];
            }

            return 'ADT^A01'; // Default fallback
        } catch (error) {
            console.warn('Could not extract message type:', error);
            return 'ADT^A01';
        }
    }

    /**
     * Use sample HL7 message
     */
    useSampleHL7Message(messageType = 'ADT^A01') {
        const sampleMessages = {
            'ADT^A01': `MSH|^~\\&|HIS|RIH|EKG|EKG~ECG~EKG2|199904140038||ADT^A01|12345|P|2.5
PID|1||123456||DOE^JOHN^||19800101|M|||123 MAIN ST^^ANYTOWN^NY^12345|||||S||123456789|123456789
PV1|1|I|2000^2012^01||||004777^ATTEND^AARON^A|||SUR|||||||004777^ATTEND^AARON^A|INP|INS|||19800101140038|||||||||||||||||||`,
            'ORU^R01': `MSH|^~\\&|LAB|RIH|EKG|EKG~ECG~EKG2|199904140038||ORU^R01|12345|P|2.5
PID|1||123456||DOE^JOHN^||19800101|M|||123 MAIN ST^^ANYTOWN^NY^12345|||||S||123456789|123456789
OBR|1|||CBC^COMPLETE BLOOD COUNT^L|||20220101080000
OBX|1|ST|WBC^White Blood Count^L||7.5|10*3/uL|4.0-11.0||||F`,
            'ADT^A03': `MSH|^~\\&|HIS|RIH|EKG|EKG~ECG~EKG2|199904140038||ADT^A03|12345|P|2.5
PID|1||123456||DOE^JOHN^||19800101|M|||123 MAIN ST^^ANYTOWN^NY^12345|||||S||123456789|123456789
PV1|1|I|2000^2012^01||||004777^ATTEND^AARON^A|||SUR|||||||004777^ATTEND^AARON^A|INP|INS|||19800101140038|||||||||||||||||||`
        };

        const sampleMessage = sampleMessages[messageType] || sampleMessages['ADT^A01'];

        this.data.hl7Message = sampleMessage;
        this.data.useSampleHL7 = true;
        this.data.detectedMessageType = messageType;
        this.data.lastModified = new Date().toISOString();

        // Emit sample loaded event
        this.dispatchEvent(new CustomEvent('sampleHL7Loaded', {
            detail: {
                messageType: messageType,
                message: sampleMessage
            }
        }));

        return sampleMessage;
    }

    /**
     * Import data (for loading saved configurations)
     */
    importData(data) {
        this.data = { ...this.data, ...data };
        this.data.lastModified = new Date().toISOString();

        this.dispatchEvent(new CustomEvent('dataImported', {
            detail: { data: this.data }
        }));
    }

    /**
     * Export data (for saving/backup)
     */
    exportData() {
        return JSON.parse(JSON.stringify(this.data));
    }
}

// Make available globally
window.WizardModel = WizardModel;