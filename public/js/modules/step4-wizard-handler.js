// ============================================================================
// Step 4 Wizard Handler - Complete Integration
// File: public/js/modules/step4-wizard-handler.js
// ============================================================================

/**
 * Enhanced Step 4 Handler for FHIR Resource Mapping
 * Integrates with the existing wizard system architecture
 */
class FHIRMappingStepHandler extends BaseStepHandler {
    constructor(wizard) {
        super(wizard);
        this.fhirResourceIntegration = null;
        this.currentMappings = new Map();
        this.availableResources = [];
        this.templateLoaded = false;
        this.initialized = false;
    }

    async initialize() {
        console.log('🎯 Initializing FHIR Mapping Step 4...');
        
        try {
            // Load the step 4 template first
            await this.loadStep4Template();
            
            // Initialize FHIR resource integration
            await this.initializeFHIRIntegration();
            
            // Load FHIR resources from parsed data
            await this.loadFHIRResourcesFromParsedData();
            
            // Setup the mapping interface
            this.setupMappingInterface();
            
            this.initialized = true;
            console.log('✅ Step 4 FHIR Mapping initialization complete');
        } catch (error) {
            console.error('❌ Step 4 initialization failed:', error);
            this.loadErrorState(error);
        }
    }

    async loadStep4Template() {
        const step4Element = document.getElementById('step4');
        if (!step4Element) {
            throw new Error('Step 4 element not found');
        }

        try {
            // Try to load the enhanced template
            const response = await fetch('/templates/step4-fhir-mapping.html');
            if (response.ok) {
                const html = await response.text();
                step4Element.innerHTML = html;
                this.templateLoaded = true;
                console.log('✅ Step 4 enhanced template loaded');
            } else {
                throw new Error('Template not found, using fallback');
            }
        } catch (error) {
            console.warn('⚠️ Enhanced template not available, using integrated fallback');
            this.loadIntegratedTemplate();
        }
    }

    loadIntegratedTemplate() {
        const step4Element = document.getElementById('step4');
        step4Element.innerHTML = `
            <div class="step-header">
                <div class="step-progress">STEP 4 OF 5</div>
                <h3 class="step-title">FHIR Resource Mapping</h3>
                <p class="step-description">
                    <img src="/assets/logos/ezHealthKonnect.jpeg" alt="ezHealthKonnect" style="width: 16px; height: 16px; border-radius: 2px; margin-right: 6px;">
                    Configure HL7 to FHIR resource mappings
                </p>
            </div>

            <!-- FHIR Resource Overview -->
            <div id="fhir-resource-overview" class="fhir-resource-overview">
                <div class="loading-state" style="text-align: center; padding: 40px; color: #6b7280;">
                    <div class="loading-spinner" style="margin: 0 auto 16px auto; width: 32px; height: 32px; border: 3px solid #f3f4f6; border-top: 3px solid #1e3a8a; border-radius: 50%; animation: spin 1s linear infinite;"></div>
                    <div style="font-weight: 600; margin-bottom: 8px;">Loading FHIR Resources</div>
                    <div style="font-size: 14px;">Analyzing HL7 message structure...</div>
                </div>
            </div>

            <!-- Mapping Strategy Controls -->
            <div class="mapping-strategy" style="display: flex; justify-content: space-between; align-items: center; background: linear-gradient(135deg, #f8f9ff 0%, #fff 100%); border: 2px solid #f8bbd9; border-radius: 12px; padding: 16px 20px; margin-bottom: 24px;">
                <div class="strategy-tabs" style="display: flex; gap: 2px; background: #e5e7eb; border-radius: 8px; padding: 2px;">
                    <button class="strategy-tab active" data-strategy="resource" style="padding: 8px 16px; border: none; background: #1e3a8a; color: white; border-radius: 6px; font-size: 13px; font-weight: 500; cursor: pointer;">By FHIR Resource</button>
                    <button class="strategy-tab" data-strategy="required" style="padding: 8px 16px; border: none; background: none; color: #6b7280; border-radius: 6px; font-size: 13px; font-weight: 500; cursor: pointer;">Required First</button>
                    <button class="strategy-tab" data-strategy="auto" style="padding: 8px 16px; border: none; background: none; color: #6b7280; border-radius: 6px; font-size: 13px; font-weight: 500; cursor: pointer;">AI Suggestions</button>
                </div>
                
                <div class="strategy-actions" style="display: flex; gap: 12px;">
                    <button id="generateAllBtn" class="strategy-btn primary" style="padding: 10px 16px; border: none; border-radius: 8px; font-size: 14px; font-weight: 600; cursor: pointer; background: #1e3a8a; color: white; display: flex; align-items: center; gap: 6px;">
                        🚀 Auto-Generate Mappings
                    </button>
                    <button id="validateBtn" class="strategy-btn secondary" style="padding: 10px 16px; border: 2px solid #f8bbd9; border-radius: 8px; font-size: 14px; font-weight: 600; cursor: pointer; background: white; color: #1e3a8a; display: flex; align-items: center; gap: 6px;">
                        ✅ Validate FHIR
                    </button>
                </div>
            </div>

            <!-- FHIR Resources Container -->
            <div id="fhir-resources-container" class="fhir-resources-container" style="display: flex; flex-direction: column; gap: 16px;">
                <!-- Will be populated by initialization -->
            </div>

            <!-- Mapping Summary -->
            <div class="mapping-summary" style="background: white; border: 2px solid #f8bbd9; border-radius: 12px; padding: 20px; margin-top: 24px; display: flex; justify-content: space-between; align-items: center;">
                <div class="summary-stats" style="display: flex; gap: 24px;">
                    <div class="summary-stat" style="text-align: center;">
                        <div id="totalMappings" class="stat-number" style="font-size: 24px; font-weight: 700; color: #1e3a8a; line-height: 1; margin-bottom: 4px;">0</div>
                        <div class="stat-label" style="font-size: 12px; color: #6b7280; text-transform: uppercase; letter-spacing: 0.5px;">Total Mappings</div>
                    </div>
                    <div class="summary-stat" style="text-align: center;">
                        <div id="requiredMapped" class="stat-number" style="font-size: 24px; font-weight: 700; color: #1e3a8a; line-height: 1; margin-bottom: 4px;">0/0</div>
                        <div class="stat-label" style="font-size: 12px; color: #6b7280; text-transform: uppercase; letter-spacing: 0.5px;">Required Mapped</div>
                    </div>
                    <div class="summary-stat" style="text-align: center;">
                        <div id="completionPercentage" class="stat-number" style="font-size: 24px; font-weight: 700; color: #1e3a8a; line-height: 1; margin-bottom: 4px;">0%</div>
                        <div class="stat-label" style="font-size: 12px; color: #6b7280; text-transform: uppercase; letter-spacing: 0.5px;">Completion</div>
                    </div>
                </div>
                
                <div class="summary-actions" style="display: flex; gap: 12px;">
                    <button id="previewFHIRBtn" class="summary-btn primary" style="padding: 10px 16px; border: none; border-radius: 8px; font-size: 14px; font-weight: 600; cursor: pointer; background: #1e3a8a; color: white; display: flex; align-items: center; gap: 6px;">
                        👁️ Preview FHIR
                    </button>
                    <button id="exportMappingsBtn" class="summary-btn secondary" style="padding: 10px 16px; border: 2px solid #f8bbd9; border-radius: 8px; font-size: 14px; font-weight: 600; cursor: pointer; background: white; color: #1e3a8a; display: flex; align-items: center; gap: 6px;">
                        📤 Export Config
                    </button>
                </div>
            </div>

            <style>
            @keyframes spin {
                0% { transform: rotate(0deg); }
                100% { transform: rotate(360deg); }
            }
            
            .strategy-tab:hover, .strategy-btn:hover, .summary-btn:hover {
                transform: translateY(-1px);
                transition: all 0.2s ease;
            }
            
            .strategy-tab:not(.active):hover {
                background: rgba(30, 58, 138, 0.1);
                color: #1e3a8a;
            }
            </style>
        `;

        this.templateLoaded = true;
        console.log('✅ Integrated Step 4 template loaded');
    }

    async initializeFHIRIntegration() {
        // Initialize the enhanced FHIR resource integration
        if (window.fhirResourceIntegration) {
            this.fhirResourceIntegration = window.fhirResourceIntegration;
            console.log('✅ Using existing FHIR resource integration');
        } else if (window.EnhancedFHIRResourceMappingIntegration) {
            this.fhirResourceIntegration = new window.EnhancedFHIRResourceMappingIntegration();
            console.log('✅ Created new enhanced FHIR resource integration');
        } else if (window.FHIRResourceMappingIntegration) {
            this.fhirResourceIntegration = new window.FHIRResourceMappingIntegration();
            console.log('✅ Created new FHIR resource integration');
        } else {
            console.warn('⚠️ No FHIR resource integration available, using basic implementation');
            this.fhirResourceIntegration = this.createBasicIntegration();
        }
    }

    createBasicIntegration() {
        return {
            initializeForStep4: async (messageType, parsedData, interfaceId) => {
                const fallbackResources = this.getFallbackResources(messageType);
                return {
                    success: true,
                    fhirResources: fallbackResources,
                    resourceLogic: {},
                    source: 'BASIC_FALLBACK',
                    messageType: messageType
                };
            },
            updateStep4Interface: () => {},
            showResourceDetails: () => {
                alert('Basic integration - resource details not available');
            }
        };
    }

    async loadFHIRResourcesFromParsedData() {
        const messageType = this.getMessageTypeFromParsedData();
        const parsedHL7 = this.getParsedHL7DataForAPI();
        const interfaceId = this.wizard.wizardData.interfaceId || this.wizard.wizardData.interfaceName;

        console.log(`🔍 Loading FHIR resources for ${messageType} with interface ${interfaceId}`);

        try {
            const result = await this.fhirResourceIntegration.initializeForStep4(
                messageType, 
                parsedHL7, 
                interfaceId
            );
            
            if (result && result.fhirResources) {
                this.availableResources = result.fhirResources;
                
                // Store the result in wizard data for other steps
                this.wizard.wizardData.step4Data = {
                    messageType: messageType,
                    fhirResources: result.fhirResources,
                    resourceLogic: result.resourceLogic || {},
                    resourceSource: result.source || 'API',
                    interfaceId: interfaceId
                };
                
                console.log(`✅ Loaded ${this.availableResources.length} FHIR resources`);
                this.updateResourceDisplay(result);
            } else {
                throw new Error('No resources returned from API');
            }
        } catch (error) {
            console.error('❌ Error loading FHIR resources:', error);
            this.handleResourceLoadError(messageType, error);
        }
    }

    getMessageTypeFromParsedData() {
        // Multiple strategies to extract message type
        
        // Strategy 1: From wizard data
        if (this.wizard.wizardData.detectedMessageType) {
            return this.wizard.wizardData.detectedMessageType;
        }
        
        // Strategy 2: From parsed HL7 data structure
        if (this.wizard.parsedHL7Data?.data?.messageType?.name) {
            return this.wizard.parsedHL7Data.data.messageType.name;
        }
        
        // Strategy 3: From MSH segment
        const msh = this.wizard.parsedHL7Data?.data?.enhancedSegments?.MSH;
        if (msh?.fields?.['MSH.9']?.value) {
            return msh.fields['MSH.9'].value;
        }
        
        // Strategy 4: From basic segments
        const basicMSH = this.wizard.parsedHL7Data?.data?.basicSegments?.MSH;
        if (basicMSH?.fields?.['MSH.9']) {
            return basicMSH.fields['MSH.9'];
        }
        
        console.warn('⚠️ Could not determine message type, using default');
        return 'ADT^A01'; // Default fallback
    }

    getParsedHL7DataForAPI() {
        // Convert the wizard's parsed HL7 format to the format expected by the backend
        const enhancedSegments = this.wizard.parsedHL7Data?.data?.enhancedSegments || {};
        const basicSegments = this.wizard.parsedHL7Data?.data?.basicSegments || {};
        
        const converted = {};
        
        // Try enhanced segments first
        if (Object.keys(enhancedSegments).length > 0) {
            Object.entries(enhancedSegments).forEach(([segmentName, segment]) => {
                converted[segmentName] = {};
                
                if (segment.fields) {
                    Object.entries(segment.fields).forEach(([fieldKey, fieldData]) => {
                        const fieldNumber = fieldKey.split('.')[1];
                        converted[segmentName][fieldNumber] = fieldData.value || '';
                    });
                }
            });
        }
        // Fallback to basic segments
        else if (Object.keys(basicSegments).length > 0) {
            Object.entries(basicSegments).forEach(([segmentName, segment]) => {
                converted[segmentName] = segment.fields || {};
            });
        }
        
        console.log('🔄 Converted HL7 data for API:', converted);
        return converted;
    }

    updateResourceDisplay(resourceData) {
        const overviewContainer = document.getElementById('fhir-resource-overview');
        const resourcesContainer = document.getElementById('fhir-resources-container');
        
        if (overviewContainer) {
            // Update the overview section
            overviewContainer.innerHTML = this.createResourceOverviewHTML(resourceData);
        }
        
        if (resourcesContainer) {
            // Update the resources container
            resourcesContainer.innerHTML = this.createResourceCardsHTML(resourceData);
        }
        
        this.updateSummaryStats();
    }

    createResourceOverviewHTML(resourceData) {
        const requiredCount = Object.values(resourceData.resourceLogic || {})
            .filter(config => config.required).length;
        const conditionalCount = resourceData.fhirResources.length - requiredCount;

        return `
            <div style="background: linear-gradient(135deg, #f8f9ff 0%, #fff 100%); border: 2px solid #f8bbd9; border-radius: 12px; padding: 16px 20px;">
                <div style="display: flex; justify-content: space-between; align-items: center;">
                    <div>
                        <h4 style="margin: 0 0 6px 0; font-size: 16px; font-weight: 600; color: #1e3a8a;">
                            🎯 FHIR Resources for ${resourceData.messageType}
                        </h4>
                        <div style="display: flex; align-items: center; gap: 12px; font-size: 12px; color: #6b7280;">
                            <span style="background: #e0e7ff; color: #1e3a8a; padding: 2px 8px; border-radius: 6px; font-weight: 500;">
                                ${resourceData.fhirResources.length} Resources
                            </span>
                            <span style="background: #fef2f2; color: #dc2626; padding: 2px 8px; border-radius: 6px; font-weight: 500;">
                                ${requiredCount} Required
                            </span>
                            <span style="background: #f0fdf4; color: #16a34a; padding: 2px 8px; border-radius: 6px; font-weight: 500;">
                                ${conditionalCount} Optional
                            </span>
                            <span style="color: #d1d5db;">•</span>
                            <span style="font-style: italic;">Source: ${resourceData.source || 'API'}</span>
                        </div>
                    </div>
                    <div>
                        <button onclick="window.step4Handler.showResourceDetails()" 
                                style="background: #1e3a8a; color: white; border: none; padding: 8px 16px; border-radius: 6px; font-size: 13px; cursor: pointer;">
                            📋 Details
                        </button>
                    </div>
                </div>
            </div>
        `;
    }

    createResourceCardsHTML(resourceData) {
        return `
            <div style="display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: 12px;">
                ${resourceData.fhirResources.map(resource => this.createResourceCardHTML(resource, resourceData.resourceLogic)).join('')}
            </div>
        `;
    }

    createResourceCardHTML(resourceType, resourceLogic) {
        const config = resourceLogic[resourceType] || {};
        const isRequired = config.required || false;
        const priority = config.priority || 5;
        const statusColor = isRequired ? '#dc2626' : '#16a34a';
        const statusText = isRequired ? 'Required' : 'Optional';

        return `
            <div style="background: white; border: 1px solid #e5e7eb; border-radius: 8px; padding: 12px; cursor: pointer; transition: all 0.2s ease;"
                 onmouseover="this.style.boxShadow='0 4px 12px rgba(0,0,0,0.1)'; this.style.transform='translateY(-2px)'"
                 onmouseout="this.style.boxShadow='none'; this.style.transform='translateY(0)'"
                 onclick="window.step4Handler.selectResource('${resourceType}')">
                <div style="display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 8px;">
                    <h5 style="margin: 0; font-size: 14px; font-weight: 600; color: #1e3a8a;">${resourceType}</h5>
                    <span style="background: ${statusColor}; color: white; padding: 2px 6px; border-radius: 4px; font-size: 10px; font-weight: 500;">
                        ${statusText}
                    </span>
                </div>
                <div style="font-size: 11px; color: #6b7280; margin-bottom: 4px;">Priority ${priority}</div>
                ${config.from ? `<div style="font-size: 11px; color: #8b5cf6; font-family: monospace;">From: ${config.from}</div>` : ''}
                <div style="margin-top: 8px;">
                    <button style="background: #f8bbd9; color: #1e3a8a; border: none; padding: 4px 8px; border-radius: 4px; font-size: 10px; font-weight: 500; width: 100%;">
                        Configure Mapping
                    </button>
                </div>
            </div>
        `;
    }

    setupMappingInterface() {
        // Set up event listeners for the mapping interface
        this.setupEventListeners();
        
        // Initialize the mapping manager if available
        if (window.fhirMappingManager) {
            window.fhirMappingManager.hl7Structure = this.wizard.parsedHL7Data?.data?.enhancedSegments;
            window.fhirMappingManager.messageType = this.getMessageTypeFromParsedData();
        }
        
        // Make this handler available globally for UI interactions
        window.step4Handler = this;
        
        console.log('✅ Mapping interface setup complete');
    }

    setupEventListeners() {
        // Set up strategy button listeners
        document.querySelectorAll('.strategy-tab').forEach(tab => {
            tab.addEventListener('click', () => this.switchStrategy(tab.dataset.strategy));
        });
        
        // Set up action button listeners
        const generateBtn = document.getElementById('generateAllBtn');
        if (generateBtn) {
            generateBtn.addEventListener('click', () => this.generateAllMappings());
        }
        
        const validateBtn = document.getElementById('validateBtn');
        if (validateBtn) {
            validateBtn.addEventListener('click', () => this.validateMappings());
        }
        
        const previewBtn = document.getElementById('previewFHIRBtn');
        if (previewBtn) {
            previewBtn.addEventListener('click', () => this.previewFHIR());
        }
        
        const exportBtn = document.getElementById('exportMappingsBtn');
        if (exportBtn) {
            exportBtn.addEventListener('click', () => this.exportMappings());
        }
        
        console.log('✅ Event listeners setup complete');
    }

    handleResourceLoadError(messageType, error) {
        console.error('❌ Failed to load FHIR resources:', error);
        
        // Use fallback resources
        this.availableResources = this.getFallbackResources(messageType);
        
        // Update display with error state
        const overviewContainer = document.getElementById('fhir-resource-overview');
        if (overviewContainer) {
            overviewContainer.innerHTML = `
                <div style="background: #fef2f2; border: 2px solid #fecaca; border-radius: 12px; padding: 16px 20px;">
                    <div style="color: #dc2626; font-weight: 600; margin-bottom: 8px;">⚠️ Could not load FHIR resources from server</div>
                    <div style="color: #6b7280; font-size: 14px; margin-bottom: 12px;">Using fallback resources for ${messageType}</div>
                    <div style="display: flex; gap: 8px;">
                        ${this.availableResources.map(resource => 
                            `<span style="background: #f3f4f6; padding: 4px 8px; border-radius: 4px; font-size: 12px;">${resource}</span>`
                        ).join('')}
                    </div>
                </div>
            `;
        }
        
        // Store fallback data
        this.wizard.wizardData.step4Data = {
            messageType: messageType,
            fhirResources: this.availableResources,
            resourceLogic: {},
            resourceSource: 'FALLBACK',
            error: error.message
        };
    }

    getFallbackResources(messageType) {
        const fallbacks = {
            'ADT^A01': ['Bundle', 'MessageHeader', 'Patient', 'Encounter'],
            'ADT^A04': ['Bundle', 'MessageHeader', 'Patient', 'Encounter'],
            'ORU^R01': ['Bundle', 'MessageHeader', 'Patient', 'DiagnosticReport', 'Observation'],
            'ORM^O01': ['Bundle', 'MessageHeader', 'Patient', 'ServiceRequest'],
            'default': ['Bundle', 'MessageHeader', 'Patient']
        };
        return fallbacks[messageType] || fallbacks['default'];
    }

    loadErrorState(error) {
        const step4Element = document.getElementById('step4');
        step4Element.innerHTML = `
            <div class="step-header">
                <div class="step-progress">STEP 4 OF 5</div>
                <h3 class="step-title">FHIR Resource Mapping - Error</h3>
                <p class="step-description">There was an issue loading the mapping interface</p>
            </div>
            
            <div style="background: #fef2f2; border: 2px solid #fecaca; border-radius: 12px; padding: 24px; text-align: center;">
                <div style="font-size: 48px; margin-bottom: 16px;">⚠️</div>
                <div style="font-weight: 600; color: #dc2626; margin-bottom: 8px;">Failed to Load FHIR Mapping Interface</div>
                <div style="color: #6b7280; margin-bottom: 16px;">${error.message}</div>
                <button onclick="window.step4Handler.initialize()" 
                        style="background: #1e3a8a; color: white; border: none; padding: 10px 20px; border-radius: 6px; cursor: pointer;">
                    🔄 Retry Initialization
                </button>
            </div>
        `;
    }

    // UI Interaction Methods
    switchStrategy(strategy) {
        document.querySelectorAll('.strategy-tab').forEach(tab => {
            tab.classList.remove('active');
            tab.style.background = 'none';
            tab.style.color = '#6b7280';
        });
        
        const activeTab = document.querySelector(`[data-strategy="${strategy}"]`);
        if (activeTab) {
            activeTab.classList.add('active');
            activeTab.style.background = '#1e3a8a';
            activeTab.style.color = 'white';
        }
        
        console.log(`🔄 Switched to strategy: ${strategy}`);
    }

    selectResource(resourceType) {
        console.log(`🎯 Selected resource: ${resourceType}`);
        
        // Visual feedback
        document.querySelectorAll('[onclick*="selectResource"]').forEach(card => {
            card.style.border = '1px solid #e5e7eb';
        });
        
        event.target.closest('div').style.border = '2px solid #f8bbd9';
        
        // Show mapping options for this resource
        this.showNotification(`Selected ${resourceType} for mapping configuration`, 'info');
    }

    showResourceDetails() {
        const details = this.availableResources.map((resource, index) => 
            `${index + 1}. ${resource}`
        ).join('\n');
        
        alert(`Available FHIR Resources:\n\n${details}\n\nTotal: ${this.availableResources.length} resources\nMessage Type: ${this.getMessageTypeFromParsedData()}`);
    }

    async generateAllMappings() {
        this.showNotification('Generating AI-powered mappings...', 'info');
        
        try {
            if (window.step4API) {
                const messageType = this.getMessageTypeFromParsedData();
                const hl7Data = this.getParsedHL7DataForAPI();
                
                const result = await window.step4API.getMappingSuggestions(hl7Data, messageType);
                
                if (result.success && result.suggestions) {
                    this.applyMappingSuggestions(result.suggestions);
                    this.showNotification(`Generated ${result.suggestions.length} mapping suggestions`, 'success');
                } else {
                    this.showNotification('No mapping suggestions available', 'warning');
                }
            }
        } catch (error) {
            console.error('❌ Error generating mappings:', error);
            this.showNotification('Failed to generate mappings', 'error');
        }
    }

    applyMappingSuggestions(suggestions) {
        suggestions.forEach(suggestion => {
            if (suggestion.confidence > 0.7) {
                this.currentMappings.set(suggestion.fhir_path, {
                    hl7Field: suggestion.hl7_field,
                    transformType: suggestion.transform_type || 'direct',
                    confidence: suggestion.confidence
                });
            }
        });
        
        this.updateSummaryStats();
    }

    validateMappings() {
        const mappingCount = this.currentMappings.size;
        const requiredMappings = 3; // Minimum required
        
        if (mappingCount >= requiredMappings) {
            this.showNotification('✅ Mapping validation passed', 'success');
        } else {
            this.showNotification(`⚠️ Please configure at least ${requiredMappings} mappings`, 'warning');
        }
    }

    async previewFHIR() {
        this.showNotification('Generating FHIR preview...', 'info');
        
        try {
            if (window.step4API && this.currentMappings.size > 0) {
                const mappings = Array.from(this.currentMappings.entries()).map(([fhirPath, mapping]) => ({
                    fhir_path: fhirPath,
                    hl7_field: mapping.hl7Field,
                    transform_type: mapping.transformType
                }));
                
                const result = await window.step4API.previewTransformation(
                    this.wizard.uploadedFileContent || '', 
                    mappings
                );
                
                if (result.success) {
                    this.showNotification('FHIR preview generated successfully', 'success');
                    console.log('FHIR Preview:', result);
                } else {
                    this.showNotification('Failed to generate FHIR preview', 'error');
                }
            } else {
                this.showNotification('No mappings configured for preview', 'warning');
            }
        } catch (error) {
            console.error('❌ Error generating preview:', error);
            this.showNotification('Preview generation failed', 'error');
        }
    }

    exportMappings() {
        const exportData = {
            messageType: this.getMessageTypeFromParsedData(),
            interfaceId: this.wizard.wizardData.interfaceId,
            fhirResources: this.availableResources,
            mappings: Object.fromEntries(this.currentMappings),
            timestamp: new Date().toISOString()
        };
        
        const blob = new Blob([JSON.stringify(exportData, null, 2)], { type: 'application/json' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `fhir-mapping-${exportData.messageType.replace('^', '_')}-${Date.now()}.json`;
        a.click();
        URL.revokeObjectURL(url);
        
        this.showNotification('Mapping configuration exported', 'success');
    }

    updateSummaryStats() {
        const totalMappings = this.currentMappings.size;
        const requiredMappings = Math.min(3, this.availableResources.length);
        const completionPercentage = requiredMappings > 0 ? Math.round((totalMappings / requiredMappings) * 100) : 0;
        
        const totalEl = document.getElementById('totalMappings');
        const requiredEl = document.getElementById('requiredMapped');
        const completionEl = document.getElementById('completionPercentage');
        
        if (totalEl) totalEl.textContent = totalMappings;
        if (requiredEl) requiredEl.textContent = `${Math.min(totalMappings, requiredMappings)}/${requiredMappings}`;
        if (completionEl) completionEl.textContent = `${completionPercentage}%`;
    }

    // BaseStepHandler interface implementation
    validate() {
        if (!this.initialized) {
            this.showNotification('Step 4 not fully initialized', 'error');
            return false;
        }
        
        const minimumMappings = 2; // At least Patient and MessageHeader
        
        if (this.currentMappings.size < minimumMappings) {
            this.showNotification(`Please configure at least ${minimumMappings} FHIR resource mappings to proceed`, 'warning');
            return false;
        }
        
        return true;
    }

    reset() {
        this.currentMappings.clear();
        this.availableResources = [];
        this.templateLoaded = false;
        this.initialized = false;
        
        // Clear wizard data
        if (this.wizard.wizardData.step4Data) {
            delete this.wizard.wizardData.step4Data;
        }
        
        console.log('✅ Step 4 reset complete');
    }

    getStepData() {
        return {
            messageType: this.getMessageTypeFromParsedData(),
            fhirResources: this.availableResources,
            mappings: Object.fromEntries(this.currentMappings),
            initialized: this.initialized,
            templateLoaded: this.templateLoaded
        };
    }

    showNotification(message, type = 'info') {
        // Use wizard's notification system if available
        if (this.wizard && this.wizard.showNotification) {
            this.wizard.showNotification(message, type);
        } else {
            console.log(`${type.toUpperCase()}: ${message}`);
            
            // Simple toast notification
            const toast = document.createElement('div');
            toast.style.cssText = `
                position: fixed; top: 20px; right: 20px; z-index: 10000;
                padding: 12px 16px; border-radius: 6px; color: white; font-size: 14px;
                background: ${type === 'success' ? '#22c55e' : type === 'error' ? '#ef4444' : type === 'warning' ? '#f59e0b' : '#3b82f6'};
            `;
            toast.textContent = message;
            document.body.appendChild(toast);
            
            setTimeout(() => toast.remove(), 3000);
        }
    }
}

// Global registration for the wizard system
if (typeof window !== 'undefined') {
    window.FHIRMappingStepHandler = FHIRMappingStepHandler;
    console.log('✅ FHIRMappingStepHandler registered globally');
}

// Export for module systems
if (typeof module !== 'undefined' && module.exports) {
    module.exports = FHIRMappingStepHandler;
}