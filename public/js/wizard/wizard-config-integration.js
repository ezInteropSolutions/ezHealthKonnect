// public/js/wizard/wizard-config-integration.js
// Client-side integration for your existing ezHealthKonnect wizard
// OPTIMIZED VERSION with enhanced error handling and best practices

/**
 * Interface Configuration Manager
 * Works with your existing Node.js app and Go backend proxy
 */
class InterfaceConfigManager {
    constructor(wizard) {
        this.wizard = wizard;
        this.nodeBackendUrl = window.location.origin;
        this.requestTimeout = 30000; // 30 second timeout
        this.retryAttempts = 3;
    }

    /**
     * Save configuration after Step 4 completion
     * Calls your new /api/wizard/save-config endpoint
     */
    async saveConfigurationAfterStep4() {
        try {
            console.log('📄 Starting interface configuration save...');
            
            // Collect configuration from wizard
            const wizardData = this.collectWizardConfiguration();
            
            // Validate required data
            if (!this.validateConfiguration(wizardData)) {
                throw new Error('Configuration validation failed');
            }
            
            // Show progress indicator
            this.showLoading('Saving Configuration...', 'Please wait while we save your interface configuration');
            
            // Send to your Node.js backend (which proxies to Go)
            const result = await this.sendToBackendWithRetry('/api/wizard/save-config', {
                wizardData: wizardData
            });
            
            // Store interface ID for later use
            if (this.wizard.wizardData && result.data?.interfaceId) {
                this.wizard.wizardData.interfaceId = result.data.interfaceId;
            }
            
            // Hide loading indicator
            this.hideLoading();
            
            // Show success notification
            this.showNotification('Configuration saved successfully!', 'success');
            
            console.log('Interface configuration saved:', result.data);
            return result;
            
        } catch (error) {
            console.error('Failed to save configuration:', error);
            this.hideLoading();
            this.showNotification(`Failed to save configuration: ${error.message}`, 'error');
            throw error;
        }
    }

    /**
     * CRITICAL: Enhanced completeWizard method to include Step 4 data
     * This replaces the existing completeWizard method
     */
    async completeWizard() {
        try {
            console.log('🏁 Completing wizard with enhanced data collection...');
            
            // STEP 1: Collect ALL wizard configuration including Step 4 data
            const wizardData = this.collectWizardConfiguration();
            
            // STEP 2: CRITICAL - Add Step 4 mapping data explicitly
            const step4Handler = this.wizard.stepHandlers?.[4] || window.step4Handler;
            if (step4Handler) {
                console.log('📊 Adding Step 4 mapping data to wizard completion...');
                
                // Add Step 4 data to the wizard data
                wizardData.atomicMappings = step4Handler.atomicMappings || [];
                wizardData.fhirTransformResult = step4Handler.fhirTransformResult || null;
                wizardData.transformationMapping = step4Handler.transformationMapping || {};
                wizardData.parsedMessage = step4Handler.parsedMessage || this.wizard.parsedHL7Data?.data;
                wizardData.enhancedSegments = this.wizard.parsedHL7Data?.data?.enhancedSegments || {};
                
                console.log('✅ Step 4 data added:', {
                    atomicMappingsCount: wizardData.atomicMappings.length,
                    hasFhirTransformResult: !!wizardData.fhirTransformResult,
                    hasParsedMessage: !!wizardData.parsedMessage,
                    segmentCount: Object.keys(wizardData.enhancedSegments).length
                });
            } else {
                console.warn('⚠️ Step 4 handler not found - mapping data may be missing');
            }
            
            // STEP 3: Validate configuration
            if (!this.validateConfiguration(wizardData)) {
                throw new Error('Configuration validation failed');
            }
            
            console.log('✅ Configuration validation passed');
            
            // STEP 4: Show progress and send to backend
            this.showLoading('Creating Interface...', 'Saving configuration and activating interface...');
            
            const interfaceResult = await this.sendToBackendWithRetry('/api/wizard/complete', {
                wizardData: wizardData
            });
            
            // Hide loading indicator
            this.hideLoading();
            
            // Show success notification
            this.showNotification('🎉 Interface created and activated successfully!', 'success');
            
            console.log('✅ Wizard completed:', interfaceResult.data);
            return interfaceResult;
            
        } catch (error) {
            console.error('❌ Failed to complete wizard:', error);
            this.hideLoading();
            this.showNotification(`Failed to complete wizard: ${error.message}`, 'error');
            throw error;
        }
    }

    async saveMappingConfiguration(interfaceId, wizardData) {
        try {
            console.log('💾 Saving HL7-FHIR mapping configuration to database...');
            
            if (!interfaceId) {
                throw new Error('Interface ID is required for mapping save');
            }
            
            // Extract Step 4 mapping data
            const step4Handler = this.wizard.stepHandlers?.[4] || window.step4Handler;
            
            if (!step4Handler) {
                console.warn('⚠️ Step 4 handler not found, mapping data may be incomplete');
            }
            
            // Prepare mapping data for database save
            const mappingData = {
                interfaceId: interfaceId,
                wizardData: {
                    detectedMessageType: wizardData.messageType || 
                                       this.wizard.parsedHL7Data?.data?.messageType?.name ||
                                       this.wizard.wizardData?.detectedMessageType,
                    
                    enhancedSegments: this.wizard.parsedHL7Data?.data?.enhancedSegments || {},
                    
                    // CRITICAL: Extract atomic mappings from Step 4 handler
                    atomicMappings: step4Handler?.atomicMappings || [],
                    
                    // Extract FHIR transformation result
                    fhirTransformResult: step4Handler?.fhirTransformResult || null,
                    
                    // Include parsed message data
                    parsedMessage: this.wizard.parsedHL7Data?.data || null
                }
            };
            
            console.log('📊 Mapping data to save:', {
                interfaceId: mappingData.interfaceId,
                messageType: mappingData.wizardData.detectedMessageType,
                atomicMappingsCount: mappingData.wizardData.atomicMappings?.length || 0,
                segmentsCount: Object.keys(mappingData.wizardData.enhancedSegments).length
            });
            
            // Call our new mapping save endpoint
            const response = await fetch('/api/wizard/save-mapping-config', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Accept': 'application/json'
                },
                credentials: 'same-origin',
                body: JSON.stringify(mappingData)
            });
            
            if (!response.ok) {
                const errorText = await response.text();
                throw new Error(`HTTP ${response.status}: ${errorText}`);
            }
            
            const result = await response.json();
            
            if (!result.success) {
                throw new Error(result.error || 'Unknown mapping save error');
            }
            
            console.log('✅ Mapping configuration saved to database:', result.data);
            return result;
            
        } catch (error) {
            console.error('❌ Failed to save mapping configuration:', error);
            return {
                success: false,
                error: error.message
            };
        }
    }

    /**
     * Collect configuration data from all wizard steps
     * ENHANCED: Collects data from wizard state, DOM elements, and step handlers
     */
    collectWizardConfiguration() {
    console.log('📊 Collecting enhanced wizard configuration data...');
    
    try {
        // Get interface ID from wizard state
        const interfaceId = this.wizard.wizardData?.interfaceId ||
                           this.wizard.wizardData?.id;
        
        // STEP 1: Basic Configuration - Try multiple sources
        const name = this.getElementValue('wizardInterfaceName') || 
                    this.wizard.wizardData?.name || 
                    this.getElementValue('interfaceName');
        const description = this.getElementValue('wizardInterfaceDescription') || 
                           this.wizard.wizardData?.description || 
                           this.getElementValue('interfaceDescription') || '';
        
        // Source and Target Types
        const sourceType = this.getElementValue('wizardSourceType') || 
                          this.wizard.wizardData?.sourceType || 
                          this.getSelectedValue('sourceType') || 'hl7v2';
        const targetType = this.getElementValue('wizardTargetType') || 
                          this.wizard.wizardData?.targetType || 
                          this.getSelectedValue('targetType') || 'fhir';
        
        // Message Type - from Step 2 or file upload
        const messageType = this.getElementValue('wizardMessageType') || 
                           this.wizard.wizardData?.messageType || 
                           this.getDetectedMessageType() || 'ADT^A01';

        // STEP 2: Source Configuration - Enhanced collection
        const sourceConfig = this.collectSourceConfiguration(sourceType);
        
        // STEP 3: Target Configuration - Enhanced collection  
        const targetConfig = this.collectTargetConfiguration(targetType);
        
        // STEP 4: CRITICAL FIX - Extract Step 4 mapping data directly
        const step4Handler = this.wizard.stepHandlers?.[4] || 
                           this.wizard.stepHandlers?.['4'] ||
                           window.fhirMappingStepHandler ||
                           window.step4Handler;
        
        console.log('🔍 Step 4 handler found:', !!step4Handler);
        console.log('🔍 Step 4 handler type:', step4Handler?.constructor?.name);
        
        let atomicMappings = [];
        let fhirTransformResult = null;
        let enhancedSegments = {};
        let parsedMessage = null;
        
        if (step4Handler) {
            atomicMappings = step4Handler.atomicMappings || [];
            fhirTransformResult = step4Handler.fhirTransformResult || null;
            parsedMessage = step4Handler.parsedMessage || this.wizard.wizardData?.parsedMessage;
            
            console.log('✅ Step 4 data extracted:', {
                atomicMappingsCount: atomicMappings.length,
                hasFhirTransformResult: !!fhirTransformResult,
                step4HandlerType: step4Handler.constructor.name
            });
            
            if (atomicMappings.length > 0) {
                console.log('🔍 Sample atomic mapping:', atomicMappings[0]);
            } else {
                console.warn('⚠️ No atomic mappings found in Step 4 handler');
                console.log('🔍 Step 4 handler.atomicMappings:', step4Handler.atomicMappings);
            }
        } else {
            console.warn('⚠️ Step 4 handler not found - mapping data will be empty');
        }
        
        // Get enhanced segments from wizard data (populated in Step 3)
        enhancedSegments = this.wizard.wizardData?.enhancedSegments ||
                          this.wizard.parsedHL7Data?.data?.enhancedSegments || 
                          {};
        
        // Get parsed message from wizard data (populated in Step 2)
        if (!parsedMessage) {
            parsedMessage = this.wizard.wizardData?.parsedMessage ||
                           this.wizard.parsedHL7Data?.data ||
                           null;
        }
        
        // Combine all configuration
        const rawConfig = {
            // Interface ID for mapping save
            interfaceId: interfaceId,
            
            // Step 1: Basic Configuration
            name: name,
            description: description,
            sourceType: sourceType,
            targetType: targetType,
            messageType: messageType,
            
            // FIXED: Step 2 & 3 - Complete Connection Configuration
            sourceConfig: sourceConfig,
            targetConfig: targetConfig,
            
            // FIXED: Step 4 - All Mapping Data (directly extracted from Step 4 handler)
            atomicMappings: atomicMappings,
            fhirTransformResult: fhirTransformResult,
            enhancedSegments: enhancedSegments,
            parsedMessage: parsedMessage,
            
            // Additional mapping configuration
            mappingRuleIds: [],
            fhirVersion: step4Handler?.fhirVersion || this.wizard.wizardData?.fhirVersion || 'R4',
            fhirProfile: 'base',
            createBundle: step4Handler?.createBundle !== false,
            
            // Processing rules
            processingRules: this.collectProcessingRules(),
            
            // Transformation mapping for backend
            transformationMapping: {
                mappings: atomicMappings,
                rules: step4Handler?.transformationRules || [],
                profile: step4Handler?.fhirProfile || 'base',
                version: step4Handler?.fhirVersion || 'R4'
            },
            
            // Metadata
            createdAt: new Date().toISOString(),
            createdBy: this.wizard.wizardData?.userEmail || 'unknown'
        };

        const cleanConfig = this.removeEmptyValues(rawConfig);
        
        console.log('📊 Enhanced configuration collected:', {
            name: cleanConfig.name,
            sourceType: cleanConfig.sourceType,
            targetType: cleanConfig.targetType,
            messageType: cleanConfig.messageType,
            atomicMappingsCount: cleanConfig.atomicMappings?.length || 0,
            hasSourceConfig: !!cleanConfig.sourceConfig && Object.keys(cleanConfig.sourceConfig).length > 0,
            hasTargetConfig: !!cleanConfig.targetConfig && Object.keys(cleanConfig.targetConfig).length > 0,
            hasFhirTransformResult: !!cleanConfig.fhirTransformResult,
            segmentsCount: Object.keys(cleanConfig.enhancedSegments || {}).length
        });
        
        // CRITICAL: Alert if missing essential data
        if (!cleanConfig.atomicMappings || cleanConfig.atomicMappings.length === 0) {
            console.error('🚨 CRITICAL: No atomic mappings found!');
            console.log('🔍 Debug info:', {
                step4Handler: !!step4Handler,
                step4HandlerType: step4Handler?.constructor?.name,
                step4AtomicMappings: step4Handler?.atomicMappings,
                wizardStepHandlers: Object.keys(this.wizard.stepHandlers || {})
            });
        }
        
        if (!cleanConfig.sourceConfig || Object.keys(cleanConfig.sourceConfig).length === 0) {
            console.error('🚨 CRITICAL: No source configuration found!');
        }
        
        if (!cleanConfig.targetConfig || Object.keys(cleanConfig.targetConfig).length === 0) {
            console.error('🚨 CRITICAL: No target configuration found!');
        }
        
        return cleanConfig;
        
    } catch (error) {
        console.error('❌ Error collecting wizard configuration:', error);
        throw new Error('Failed to collect wizard configuration data');
    }
}


    /**
     * Collect source configuration based on source type
     */
    collectSourceConfiguration(sourceType) {
        console.log('🔧 Collecting source configuration for:', sourceType);
        
        const baseConfig = {
            type: sourceType,
            enabled: true
        };
        
        switch (sourceType) {
            case 'hl7v2':
            case 'hl7':
                return {
                    ...baseConfig,
                    connectivity: 'tcp',
                    host: this.getElementValue('sourceHost') || 
                          this.getElementValue('hl7SourceHost') || 
                          'localhost',
                    port: parseInt(this.getElementValue('sourcePort') || 
                                  this.getElementValue('hl7SourcePort') || 
                                  '2575'),
                    timeout: parseInt(this.getElementValue('sourceTimeout') || '30000'),
                    encoding: this.getElementValue('sourceEncoding') || 'UTF-8',
                    validateIncoming: this.getCheckboxValue('validateIncoming') !== false
                };
                
            case 'file':
                return {
                    ...baseConfig,
                    connectivity: 'file',
                    inputDirectory: this.getElementValue('sourceInputDir') || '/input',
                    processedDirectory: this.getElementValue('sourceProcessedDir') || '/processed',
                    errorDirectory: this.getElementValue('sourceErrorDir') || '/error',
                    filePattern: this.getElementValue('sourceFilePattern') || '*.hl7',
                    pollingInterval: parseInt(this.getElementValue('sourcePollingInterval') || '5000')
                };
                
            case 'http':
                return {
                    ...baseConfig,
                    connectivity: 'http',
                    endpoint: this.getElementValue('sourceEndpoint') || '/api/hl7/receive',
                    port: parseInt(this.getElementValue('sourceHttpPort') || '8080'),
                    method: this.getElementValue('sourceHttpMethod') || 'POST',
                    authentication: this.getElementValue('sourceAuth') || 'none'
                };
                
            default:
                return baseConfig;
        }
    }

    /**
     * Collect target configuration based on target type
     */
    collectTargetConfiguration(targetType) {
        console.log('🔧 Collecting target configuration for:', targetType);
        
        const baseConfig = {
            type: targetType,
            enabled: true
        };
        
        switch (targetType) {
            case 'fhir':
                return {
                    ...baseConfig,
                    connectivity: 'http',
                    baseUrl: this.getElementValue('targetFhirUrl') || 
                             this.getElementValue('targetHost') || 
                             'http://localhost:8080/fhir',
                    version: this.getElementValue('fhirVersion') || 'R4',
                    authentication: {
                        type: this.getElementValue('fhirAuthType') || 'none',
                        username: this.getElementValue('fhirUsername') || '',
                        password: this.getElementValue('fhirPassword') || '',
                        token: this.getElementValue('fhirToken') || ''
                    },
                    timeout: parseInt(this.getElementValue('targetTimeout') || '30000'),
                    validateOutput: this.getCheckboxValue('validateOutput') !== false,
                    createBundle: this.getCheckboxValue('createBundle') !== false
                };
                
            case 'database':
                return {
                    ...baseConfig,
                    connectivity: 'database',
                    connectionString: this.getElementValue('targetDbConnection') || '',
                    schema: this.getElementValue('targetDbSchema') || 'public',
                    table: this.getElementValue('targetDbTable') || 'fhir_resources',
                    batchSize: parseInt(this.getElementValue('targetDbBatchSize') || '100')
                };
                
            case 'file':
                return {
                    ...baseConfig,
                    connectivity: 'file',
                    outputDirectory: this.getElementValue('targetOutputDir') || '/output',
                    filePattern: this.getElementValue('targetFilePattern') || '{messageType}_{timestamp}.json',
                    createSubdirectories: this.getCheckboxValue('createSubdirs') !== false
                };
                
            default:
                return baseConfig;
        }
    }

    /**
     * Collect mapping configuration from Step 4 handler
     */
    collectMappingConfiguration() {
    console.log('🗂️ Collecting mapping configuration...');
    
    try {
        // CORRECTED: Get Step 4 handler from the correct location
        const step4Handler = this.wizard.stepHandlers?.[4] || 
                           this.wizard.stepHandlers?.['4'] ||
                           window.fhirMappingStepHandler ||
                           window.step4Handler;
        
        console.log('🔍 Step 4 handler found:', !!step4Handler);
        console.log('🔍 Step 4 handler type:', step4Handler?.constructor?.name);
        
        if (step4Handler) {
            // FIXED: Extract actual data from the Step 4 handler properties
            const mappingData = {
                fhirVersion: step4Handler.fhirVersion || this.wizard.wizardData?.fhirVersion || 'R4',
                createBundle: step4Handler.createBundle !== false,
                
                // CRITICAL FIX: Get atomic mappings from the correct property
                atomicMappings: step4Handler.atomicMappings || [],
                
                fhirTransformResult: step4Handler.fhirTransformResult || null,
                transformationMapping: step4Handler.transformationMapping || {},
                parsedMessage: step4Handler.parsedMessage || this.wizard.wizardData?.parsedMessage,
                resourceTypes: step4Handler.resourceTypes || [],
                fieldMappings: step4Handler.fieldMappings || []
            };
            
            console.log('✅ Mapping data collected from Step 4 handler:', {
                atomicMappingsCount: mappingData.atomicMappings.length,
                hasFhirTransformResult: !!mappingData.fhirTransformResult,
                resourceTypesCount: mappingData.resourceTypes.length,
                step4HandlerType: step4Handler.constructor.name
            });
            
            // DEBUG: Log actual atomic mappings if found
            if (mappingData.atomicMappings.length > 0) {
                console.log('🔍 Sample atomic mapping:', mappingData.atomicMappings[0]);
                console.log('🔍 All atomic mappings:', mappingData.atomicMappings);
            } else {
                console.warn('⚠️ No atomic mappings found in Step 4 handler');
                console.log('🔍 Step 4 handler properties:', Object.keys(step4Handler));
                console.log('🔍 step4Handler.atomicMappings value:', step4Handler.atomicMappings);
            }
            
            return mappingData;
        }
        
        // ENHANCED: Fallback search with comprehensive logging
        console.warn('⚠️ Step 4 handler not found, searching for mapping data...');
        
        // Search in multiple locations
        const searchResults = {
            wizardData: this.wizard.wizardData?.atomicMappings,
            globalStep4: window.step4Handler?.atomicMappings,
            globalFhir: window.fhirMappingStepHandler?.atomicMappings,
            enhancedMapping: window.enhancedMappingInterface?.atomicMappings
        };
        
        console.log('🔍 Fallback search results:', searchResults);
        
        let foundMappings = [];
        for (const [source, mappings] of Object.entries(searchResults)) {
            if (Array.isArray(mappings) && mappings.length > 0) {
                foundMappings = mappings;
                console.log(`✅ Found ${mappings.length} atomic mappings in ${source}`);
                break;
            }
        }
        
        // Fallback mapping configuration
        const fallbackMapping = {
            fhirVersion: this.wizard.wizardData?.fhirVersion || 'R4',
            createBundle: this.wizard.wizardData?.createBundle !== false,
            atomicMappings: foundMappings,
            transformationMapping: this.wizard.wizardData?.transformationMapping || {}
        };
        
        console.log('⚠️ Using fallback mapping configuration:', {
            atomicMappingsCount: fallbackMapping.atomicMappings.length
        });
        
        return fallbackMapping;
        
    } catch (error) {
        console.error('❌ Error collecting mapping configuration:', error);
        return {
            fhirVersion: 'R4',
            createBundle: true,
            atomicMappings: [],
            transformationMapping: {}
        };
    }
}


    /**
     * Collect processing rules configuration
     */
    collectProcessingRules() {
        return {
            validateInput: this.getCheckboxValue('validateInput') !== false,
            validateOutput: this.getCheckboxValue('validateOutput') !== false,
            retryFailures: this.getCheckboxValue('retryFailures') !== false,
            maxRetries: parseInt(this.getElementValue('maxRetries') || '3'),
            timeout: parseInt(this.getElementValue('processingTimeout') || '30000'),
            batchSize: parseInt(this.getElementValue('batchSize') || '1'),
            preserveOriginal: this.getCheckboxValue('preserveOriginal') !== false
        };
    }

    /**
     * Get detected message type from uploaded file
     */
    getDetectedMessageType() {
        // Try to get from Step 2 upload results
        const uploadResults = this.wizard.wizardData?.uploadResults;
        if (uploadResults?.messageType) {
            return uploadResults.messageType;
        }
        
        // Try to get from parsed message
        const parsedMessage = this.wizard.wizardData?.parsedMessage;
        if (parsedMessage?.messageType) {
            return parsedMessage.messageType;
        }
        
        // Check DOM elements
        const messageTypeElement = document.querySelector('#detectedMessageType, #messageType, .message-type');
        if (messageTypeElement) {
            return messageTypeElement.textContent || messageTypeElement.value;
        }
        
        return null;
    }

    /**
     * Get selected value from dropdown
     */
    getSelectedValue(fieldName) {
        const element = document.querySelector(`#${fieldName}, select[name="${fieldName}"]`);
        return element?.value || null;
    }

    /**
     * Get checkbox value (returns boolean)
     */
    getCheckboxValue(fieldName) {
        const element = document.querySelector(`#${fieldName}, input[name="${fieldName}"]`);
        return element?.checked || false;
    }

    getStep4MappingData(dataType) {
        try {
            const step4Handler = this.wizard.stepHandlers?.[4] || window.step4Handler;
            
            if (!step4Handler) {
                console.warn('⚠️ Step 4 handler not found, mapping data may be incomplete');
                return null;
            }
            
            switch (dataType) {
                case 'atomicMappings':
                    return step4Handler.atomicMappings || [];
                case 'fhirTransformResult':
                    return step4Handler.fhirTransformResult || null;
                default:
                    return null;
            }
        } catch (error) {
            console.error(`❌ Error getting Step 4 ${dataType}:`, error);
            return null;
        }
    }

    /**
     * Validate required configuration fields
     */
    validateConfiguration(config) {
        console.log('🔍 Validating configuration...');
        
        const requiredFields = ['name', 'sourceType', 'targetType'];
        
        for (const field of requiredFields) {
            if (!config[field] || config[field].trim() === '') {
                console.error(`❌ Validation failed: ${field} is required`);
                this.showNotification(`Missing required field: ${field}`, 'error');
                return false;
            }
        }
        
        // Additional validation
        if (config.name.length > 255) {
            console.error('❌ Validation failed: Interface name too long');
            this.showNotification('Interface name must be less than 255 characters', 'error');
            return false;
        }
        
        console.log('✅ Configuration validation passed');
        return true;
    }

    /**
     * Send request to Node.js backend with retry logic
     */
    async sendToBackendWithRetry(endpoint, data, attempt = 1) {
        try {
            console.log(`📡 Attempt ${attempt}/${this.retryAttempts} - Sending to ${endpoint}`);
            return await this.sendToBackend(endpoint, data);
        } catch (error) {
            if (attempt < this.retryAttempts && this.isRetryableError(error)) {
                console.warn(`⚠️ Request failed (attempt ${attempt}/${this.retryAttempts}), retrying...`, error.message);
                await this.delay(1000 * attempt); // Progressive delay
                return this.sendToBackendWithRetry(endpoint, data, attempt + 1);
            }
            throw error;
        }
    }

    /**
     * Send request to your Node.js backend
     */
    async sendToBackend(endpoint, data) {
        const controller = new AbortController();
        const timeoutId = setTimeout(() => controller.abort(), this.requestTimeout);
        
        try {
            const response = await fetch(endpoint, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Accept': 'application/json'
                },
                credentials: 'same-origin', // Include session cookies
                body: JSON.stringify(data),
                signal: controller.signal
            });

            clearTimeout(timeoutId);
            
            // FIXED: Log success AFTER the request completes
            console.log(`✅ Request successful on attempt ${data.attempt || 1}`);

            if (!response.ok) {
                const errorText = await response.text();
                throw new Error(`HTTP ${response.status}: ${errorText}`);
            }

            const result = await response.json();
            
            if (!result.success) {
                throw new Error(result.error || 'Unknown backend error');
            }

            return result;

        } catch (error) {
            clearTimeout(timeoutId);
            
            if (error.name === 'AbortError') {
                throw new Error('Request timeout - please try again');
            }
            
            throw error;
        }
    }

    /**
     * Helper methods
     */
    getElementValue(id) {
        const element = document.getElementById(id);
        return element ? element.value : '';
    }

    getElementChecked(id) {
        const element = document.getElementById(id);
        return element ? element.checked : false;
    }

    removeEmptyValues(obj) {
        const cleaned = {};
        for (const [key, value] of Object.entries(obj)) {
            if (value !== null && value !== undefined && value !== '') {
                if (typeof value === 'object' && !Array.isArray(value)) {
                    const cleanedNested = this.removeEmptyValues(value);
                    if (Object.keys(cleanedNested).length > 0) {
                        cleaned[key] = cleanedNested;
                    }
                } else {
                    cleaned[key] = value;
                }
            }
        }
        return cleaned;
    }

    isRetryableError(error) {
        return error.message.includes('timeout') || 
               error.message.includes('network') ||
               error.message.includes('Failed to fetch');
    }

    delay(ms) {
        return new Promise(resolve => setTimeout(resolve, ms));
    }

    showLoading(title, message) {
        if (this.wizard.showLoading) {
            this.wizard.showLoading(title, message);
        }
    }

    hideLoading() {
        if (this.wizard.hideLoading) {
            this.wizard.hideLoading();
        }
    }

    showNotification(message, type) {
        if (this.wizard.showNotification) {
            this.wizard.showNotification(message, type);
        } else {
            console.log(`${type.toUpperCase()}: ${message}`);
        }
    }
}

/**
 * Enhanced Step 4 Handler - Called when mapping is completed
 */
class EnhancedStep4Handler {
    constructor(wizard) {
        this.wizard = wizard;
        this.configManager = new InterfaceConfigManager(wizard);
    }

    /**
     * Called when step 4 mapping is completed successfully
     */
    async onMappingCompleted(mappingData) {
        try {
            console.log('🎯 Step 4 mapping completed, saving configuration...');
            
            // Store mapping data in wizard safely
            this.wizard.wizardData = this.wizard.wizardData || {};
            this.wizard.wizardData.mappingRuleIds = mappingData?.mappingRuleIds || [];
            this.wizard.wizardData.fhirVersion = mappingData?.fhirVersion || 'R4';
            this.wizard.wizardData.fhirProfile = mappingData?.fhirProfile || 'base';
            this.wizard.wizardData.createBundle = mappingData?.createBundle || false;
            
            // Save configuration
            await this.configManager.saveConfigurationAfterStep4();
            
            console.log('✅ Configuration saved successfully after step 4');
            
        } catch (error) {
            console.error('❌ Failed to save configuration after step 4:', error);
            // Don't block progression to step 5, but show warning
            this.configManager.showNotification('Warning: Configuration not saved. You can retry from the dashboard.', 'warning');
        }
    }

    /**
     * Initialize step 4 enhancements
     */
    initialize() {
        console.log('🔧 Initializing enhanced Step 4 handler...');
        console.log('✅ Enhanced Step 4 handler initialized');
    }
}

/**
 * Enhanced Wizard Main that integrates with your existing setup
 */
class EnhancedWizardMain {
    constructor(existingWizard) {
        try {
            // If you have an existing wizard, enhance it
            if (existingWizard) {
                this.wizard = existingWizard;
                this.enhanceExistingWizard();
            } else {
                // Create new wizard functionality
                this.wizard = this;
                this.initializeWizard();
            }
            
            this.configManager = new InterfaceConfigManager(this.wizard);
            this.setupConfigurationHandlers();
            
        } catch (error) {
            console.error('❌ Error initializing EnhancedWizardMain:', error);
        }
    }

    enhanceExistingWizard() {
        console.log('🔧 Enhancing existing wizard with configuration management...');
        
        try {
            // Add configuration management to existing wizard
            this.wizard.configManager = new InterfaceConfigManager(this.wizard);
            
            // FIXED: Enhanced finishWizard method with auto-refresh protection and comprehensive modal closing
            if (this.wizard.finishWizard) {
                const originalFinish = this.wizard.finishWizard.bind(this.wizard);
                this.wizard.finishWizard = async () => {
                    try {
                        console.log('🎯 Enhanced finishWizard called - attempting to save and activate...');
                        
                        // STEP 1: STOP all auto-refresh to prevent interference
                        if (window.stopAutoRefresh) {
                            window.stopAutoRefresh();
                            console.log('🛑 Auto-refresh stopped to prevent modal interference');
                        }
                        
                        // STEP 2: Complete the wizard
                        await this.configManager.completeWizard();
                        
                        // STEP 3: FORCE close modal IMMEDIATELY with comprehensive cleanup
                        console.log('🚪 Force closing modal after completion');
                        
                        // Find and close the modal
                        const overlay = document.getElementById('wizardModalOverlay');
                        if (overlay) {
                            // Remove all possible classes
                            overlay.classList.remove('show', 'maximized', 'active', 'open', 'visible');
                            
                            // Force hide with multiple methods
                            overlay.style.display = 'none !important';
                            overlay.style.visibility = 'hidden !important';
                            overlay.style.opacity = '0 !important';
                            overlay.style.pointerEvents = 'none !important';
                            overlay.style.zIndex = '-1 !important';
                            
                            // Reset body
                            document.body.style.overflow = '';
                            document.body.style.paddingRight = '';
                            document.body.classList.remove('modal-open', 'wizard-open');
                            
                            console.log('✅ Modal force hidden with comprehensive styles');
                        }
                        
                        // STEP 4: Reset ALL wizard states
                        if (window.wizardNavigation) {
                            window.wizardNavigation.isModalOpen = false;
                            window.wizardNavigation.isFinishing = false;
                            window.wizardNavigation.currentStep = 1;
                        }
                        
                        if (window.wizard) {
                            window.wizard.currentStep = 1;
                            if (typeof window.wizard.resetWizard === 'function') {
                                window.wizard.resetWizard();
                            }
                        }
                        
                        // STEP 5: Clear session storage
                        try {
                            sessionStorage.removeItem('wizardData');
                            sessionStorage.removeItem('wizardStep');
                            sessionStorage.removeItem('wizardProgress');
                        } catch (e) {
                            // Ignore storage errors
                        }
                        
                        // STEP 6: Wait a moment then restart auto-refresh and reload interfaces
                        setTimeout(() => {
                            console.log('🔄 Restarting auto-refresh and reloading interfaces');
                            
                            // Restart auto-refresh
                            if (window.startAutoRefresh) {
                                window.startAutoRefresh();
                            }
                            
                            // Reload interfaces to show the new one
                            if (typeof window.loadInterfaces === 'function') {
                                window.loadInterfaces();
                            }
                            
                            console.log('✅ Wizard cleanup complete - modal should be closed');
                        }, 500);
                        
                    } catch (error) {
                        console.error('Enhanced finish failed, trying original:', error);
                        
                        // Still try to close modal even if there's an error
                        const overlay = document.getElementById('wizardModalOverlay');
                        if (overlay) {
                            overlay.style.display = 'none !important';
                        }
                        
                        // Restart auto-refresh even on error
                        if (window.startAutoRefresh) {
                            window.startAutoRefresh();
                        }
                        
                        return originalFinish();
                    }
                };
            }
            
            // Enhance existing step handlers for Step 4
            if (this.wizard.stepHandlers?.code_summary) {
                const originalStepHandler = this.wizard.stepHandlers.code_summary;
                if (originalStepHandler && originalStepHandler.onStepComplete) {
                    this.originalOnStepComplete = originalStepHandler.onStepComplete.bind(originalStepHandler);
                    originalStepHandler.onStepComplete = async (stepNumber) => {
                        if (stepNumber === 4) {
                            // Enhanced Step 4 completion
                            await this.step4Handler.onMappingCompleted(this.wizard.wizardData);
                        }
                        // Call original completion if it exists
                        if (this.originalOnStepComplete) {
                            return this.originalOnStepComplete(stepNumber);
                        }
                    };
                }
            }
            
            console.log('✅ Enhanced wizard configuration handlers setup complete');
        } catch (error) {
            console.error('❌ Error enhancing Step 4 handler:', error);
        }
    }

    initializeWizard() {
        console.log('🔧 Initializing new enhanced wizard...');
        
        try {
            // Initialize basic wizard properties
            this.currentStep = 1;
            this.totalSteps = 5;
            this.wizardData = {};
            
            console.log('✅ New wizard initialized');
        } catch (error) {
            console.error('❌ Error initializing new wizard:', error);
        }
    }

    setupConfigurationHandlers() {
        try {
            // Set up step 4 handler
            this.step4Handler = new EnhancedStep4Handler(this.wizard);
            this.step4Handler.initialize();
            
            console.log('✅ Enhanced wizard configuration handlers setup complete');
        } catch (error) {
            console.error('❌ Error setting up configuration handlers:', error);
        }
    }

    /**
     * Helper methods delegated to configManager
     */
    collectWizardConfiguration() {
        return this.configManager.collectWizardConfiguration();
    }

    validateConfiguration(config) {
        return this.configManager.validateConfiguration(config);
    }

    showLoading(message) {
        if (this.wizard && this.wizard.showLoading) {
            this.wizard.showLoading(message);
        }
    }

    hideLoading() {
        if (this.wizard && this.wizard.hideLoading) {
            this.wizard.hideLoading();
        }
    }

    showNotification(message, type) {
        if (this.wizard && this.wizard.showNotification) {
            this.wizard.showNotification(message, type);
        } else {
            console.log(`${type.toUpperCase()}: ${message}`);
        }
    }
}

// =====================================
// INITIALIZATION WITH BETTER DETECTION
// =====================================

// Wait for wizard to be ready and then enhance it
function initializeWizardEnhancement() {
    console.log('🚀 Initializing wizard configuration integration...');
    
    try {
        // Strategy 1: Check for existing wizard instance
        if (window.wizard) {
            console.log('📦 Found existing wizard instance, enhancing...');
            window.enhancedWizardMain = new EnhancedWizardMain(window.wizard);
            console.log('✅ Wizard configuration integration initialized');
            return;
        }
        
        // Strategy 2: Check for wizard container
        if (document.getElementById('wizardModalOverlay') || document.querySelector('.wizard-modal-overlay')) {
            console.log('🎯 Found wizard container, waiting for wizard instance...');
            
            // Wait for window.wizard to be created
            let attempts = 0;
            const checkInterval = setInterval(() => {
                attempts++;
                
                if (window.wizard) {
                    clearInterval(checkInterval);
                    console.log('📦 Wizard found after waiting, enhancing...');
                    window.enhancedWizardMain = new EnhancedWizardMain(window.wizard);
                    console.log('✅ Wizard configuration integration initialized after wait');
                } else if (attempts > 50) { // 10 seconds max wait
                    clearInterval(checkInterval);
                    console.log('⏰ Timeout waiting for wizard instance');
                }
            }, 200);
            
            return;
        }
        
        // Strategy 3: Listen for wizard ready event
        window.addEventListener('wizardReady', function(event) {
            console.log('📻 Received wizardReady event, enhancing wizard...');
            const wizard = event.detail?.wizard || window.wizard;
            if (wizard) {
                window.enhancedWizardMain = new EnhancedWizardMain(wizard);
                console.log('✅ Wizard configuration integration initialized via event');
            }
        });
        
        console.log('ℹ️ No wizard found on this page yet, listeners set up for future detection');
        
    } catch (error) {
        console.error('❌ Error during wizard enhancement initialization:', error);
    }
}

// Auto-initialize when DOM is ready
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initializeWizardEnhancement);
} else {
    // DOM already loaded, try to initialize immediately
    initializeWizardEnhancement();
}

// Also try after a short delay to catch late-loading wizards
setTimeout(initializeWizardEnhancement, 1000);

// Add CSS for smooth modal transitions
const additionalCSS = `
.wizard-modal-overlay {
    transition: opacity 0.3s ease, visibility 0.3s ease !important;
}

.wizard-modal-overlay:not(.show) {
    opacity: 0 !important;
    visibility: hidden !important;
    pointer-events: none !important;
}

.wizard-btn.disabled {
    opacity: 0.6;
    cursor: not-allowed !important;
}

.spinner {
    display: inline-block;
    width: 12px;
    height: 12px;
    border: 2px solid #f3f3f3;
    border-top: 2px solid #3498db;
    border-radius: 50%;
    animation: spin 1s linear infinite;
    margin-right: 5px;
}

@keyframes spin {
    0% { transform: rotate(0deg); }
    100% { transform: rotate(360deg); }
}
`;

// Inject the CSS
const style = document.createElement('style');
style.id = 'wizard-close-spinner-styles';
style.textContent = additionalCSS;
document.head.appendChild(style);

// Enhanced debugging for wizard close mechanisms
window.debugWizardClose = function() {
    console.log('🔧 Debug: Testing wizard close mechanisms...');
    
    console.log('Available close functions:');
    console.log('- window.closeWizard:', typeof window.closeWizard);
    console.log('- window.wizardNavigation.closeWizard:', window.wizardNavigation ? typeof window.wizardNavigation.closeWizard : 'N/A');
    console.log('- window.wizard.closeModal:', window.wizard ? typeof window.wizard.closeModal : 'N/A');
    console.log('- window.enhancedWizardMain.closeWizardModal:', window.enhancedWizardMain ? typeof window.enhancedWizardMain.closeWizardModal : 'N/A');
    
    // Test direct DOM access
    const overlay = document.getElementById('wizardModalOverlay');
    console.log('- Modal overlay found:', !!overlay);
    if (overlay) {
        console.log('- Modal classes:', overlay.className);
        console.log('- Modal display:', overlay.style.display);
    }
    
    // Attempt close via enhanced wizard
    if (window.enhancedWizardMain && typeof window.enhancedWizardMain.closeWizardModal === 'function') {
        console.log('Attempting enhanced close...');
        window.enhancedWizardMain.closeWizardModal();
    } else {
        console.log('Enhanced wizard main not available, using direct DOM manipulation...');
        if (overlay) {
            overlay.style.display = 'none !important';
            overlay.classList.remove('show', 'active', 'maximized');
            document.body.style.overflow = '';
        }
    }
};

// Export for manual initialization
window.InterfaceConfigManager = InterfaceConfigManager;
window.EnhancedWizardMain = EnhancedWizardMain;

// For CommonJS environments
if (typeof module !== 'undefined' && module.exports) {
    module.exports = {
        InterfaceConfigManager,
        EnhancedStep4Handler,
        EnhancedWizardMain
    };
}

console.log('✅ Wizard Configuration Integration loaded and ready');