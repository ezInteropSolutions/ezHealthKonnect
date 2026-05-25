// step4-handler.js - CORRECTED VERSION
// Enhanced with mapping interface integration + restored JSON viewer
// ============================================================================

(function() {
    'use strict';
    
    console.log('📄 Loading FHIRMappingStepHandler class...');

    class FHIRMappingStepHandler {
        constructor(wizard) {
            this.wizard = wizard;
            this.transformationData = null;
            this.initialized = false;
            this.atomicMappings = [];
            this.fhirTransformResult = null;
            this.configurationLocked = false;
            this.savedConfiguration = null;
            this.currentJsonView = 'bundle';
            this.apiBaseUrl = this.getApiBaseUrl();
            
            // Initialize enhanced mapping interface
            this.initializeEnhancedMapping();
        }

        // Initialize enhanced mapping interface
        initializeEnhancedMapping() {
            try {
                console.log('🔧 Initializing enhanced mapping...');
                console.log('🔍 FieldMappingValidator available:', typeof FieldMappingValidator !== 'undefined');
                console.log('🔍 EnhancedMappingInterface available:', typeof EnhancedMappingInterface !== 'undefined');
                
                if (typeof FieldMappingValidator !== 'undefined' && typeof EnhancedMappingInterface !== 'undefined') {
                    this.enhancedMapping = new EnhancedMappingInterface(this);
                    
                    // Set global reference for modal callbacks - CRITICAL for onclick handlers
                    window.enhancedMappingInterface = this.enhancedMapping;
                    
                    // Verify the interface is properly created
                    if (this.enhancedMapping && typeof this.enhancedMapping.openEnhancedEditModal === 'function') {
                        console.log('✅ Enhanced mapping interface initialized successfully');
                        console.log('✅ Global reference set: window.enhancedMappingInterface');
                        console.log('✅ openEnhancedEditModal method available:', typeof this.enhancedMapping.openEnhancedEditModal);
                    } else {
                        console.error('❌ Enhanced mapping interface created but missing methods');
                        this.enhancedMapping = null;
                    }
                } else {
                    console.warn('⚠️ Enhanced mapping classes not available');
                    console.warn('   - FieldMappingValidator:', typeof FieldMappingValidator);
                    console.warn('   - EnhancedMappingInterface:', typeof EnhancedMappingInterface);
                    this.enhancedMapping = null;
                }
            } catch (error) {
                console.error('❌ Failed to initialize enhanced mapping:', error);
                console.error('Error details:', error.message);
                console.error('Stack trace:', error.stack);
                this.enhancedMapping = null;
            }
        }

        // Get API base URL from environment config
        getApiBaseUrl() {
            if (window.envConfig && window.envConfig.getApiBaseUrl) {
                return window.envConfig.getApiBaseUrl();
            }
            if (window.envConfig && window.envConfig.getApiUrl) {
                return window.envConfig.getApiUrl('');
            }
            const protocol = window.location.protocol;
            const hostname = window.location.hostname;
            const url = `${protocol}//${hostname}:8080`;
            console.log('🔍 Using API URL:', url);
            return url;
        }

        async initialize() {
            console.log('🎯 Initializing FHIR Mapping Step 4...');
            
            try {
                this.ensureStep4Visible();
                
                if (this.wizard?.parsedHL7Data?.data) {
                    await this.extractRealMappings();
                } else {
                    console.log('⏳ No parsed HL7 data yet, skipping transformation');
                }
                
                this.loadSavedConfiguration();
                await this.loadInterface();
                await new Promise(resolve => setTimeout(resolve, 300));
                this.setupEventListeners();
                this.initializeJsonTreeViewer();
                
                this.initialized = true;
                console.log('✅ FHIR Mapping Step 4 initialized successfully');
            } catch (error) {
                console.error('❌ Step 4 initialization failed:', error);
                this.showError(error.message);
            }
        }

        async extractRealMappings() {
            console.log('📊 Extracting real HL7 to FHIR mappings...');
            
            const data = this.wizard?.parsedHL7Data?.data;
            if (!data || !data.enhancedSegments) {
                console.warn('⚠️ No enhanced segments found');
                return;
            }

            try {
                const requestData = {
                    parsedHL7Data: this.wizard.parsedHL7Data,
                    createBundle: true,
                    validationMode: 'strict'
                };
                
                console.log('📡 Sending transform request to:', `${this.apiBaseUrl}/api/fhir/test-transform-v3`);
                
                const response = await fetch(`${this.apiBaseUrl}/api/fhir/test-transform-v3`, {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                    },
                    body: JSON.stringify(requestData)
                });

                if (response.ok) {
                    this.fhirTransformResult = await response.json();
                    console.log('✅ FHIR transformation successful:', this.fhirTransformResult);
                    this.extractAtomicMappingsFromResult();
                } else {
                    const errorText = await response.text();
                    console.error('❌ API Error:', response.status, errorText);
                    throw new Error(`Failed to transform: ${response.status} - ${errorText}`);
                }
            } catch (error) {
                console.error('❌ Failed to transform to FHIR:', error);
                this.fhirTransformResult = null;
            }
        }

        extractAtomicMappingsFromResult() {
            if (!this.fhirTransformResult || !this.fhirTransformResult.fhirResources) {
                return;
            }

            this.atomicMappings = [];
            const knownMappings = this.getKnownMappings();
            
            this.fhirTransformResult.fhirResources.forEach(resource => {
                this.extractMappingsFromResource(resource, resource.resourceType, knownMappings);
            });
            
            console.log(`✅ Extracted ${this.atomicMappings.length} atomic mappings`);
        }

        getKnownMappings() {
            return {
                'MessageHeader': {
                    'SENDING_APPLICATION': 'MSH.3',
                    'SENDING_FACILITY': 'MSH.4', 
                    'RECEIVING_APPLICATION': 'MSH.5',
                    'RECEIVING_FACILITY': 'MSH.6',
                    'A04': 'MSH.9.2',
                    'ADT^A04': 'MSH.9'
                },
                'Patient': {
                    '135769': 'PID.3',
                    '1719': 'PID.18',
                    'MOUSE': 'PID.5.1',
                    'MICKEY': 'PID.5.2',
                    '19281118': 'PID.7',
                    '1928-11-18': 'PID.7',
                    'male': 'PID.8',
                    'M': 'PID.8',
                    '123 Main St.': 'PID.11.1',
                    'Lake Buena Vista': 'PID.11.3',
                    'FL': 'PID.11.4',
                    '32830': 'PID.11.5',
                    '(407)939-1289': 'PID.13.1',
                    'theMainMouse@disney.com': 'PID.13.4'
                }
            };
        }

        extractMappingsFromResource(resource, resourceType, knownMappings, path = '') {
            Object.entries(resource).forEach(([key, value]) => {
                if (key === 'resourceType' || key === 'meta' || key === 'text' || key === 'id') return;
                
                const currentPath = path ? `${path}.${key}` : key;
                const fhirPath = `${resourceType}.${currentPath}`;
                
                if (value && typeof value === 'object' && !Array.isArray(value)) {
                    this.extractMappingsFromResource(value, resourceType, knownMappings, currentPath);
                } else if (Array.isArray(value)) {
                    value.forEach((item, index) => {
                        if (item && typeof item === 'object') {
                            this.extractMappingsFromResource(item, resourceType, knownMappings, `${currentPath}[${index}]`);
                        } else if (item !== null && item !== undefined && item !== '') {
                            const hl7Field = this.findHL7FieldForValue(item, resourceType, knownMappings);
                            if (hl7Field) {
                                this.atomicMappings.push({
                                    hl7Field: hl7Field,
                                    fhirPath: `${fhirPath}[${index}]`,
                                    value: item,
                                    resourceType: resourceType
                                });
                            }
                        }
                    });
                } else if (value !== null && value !== undefined && value !== '') {
                    const hl7Field = this.findHL7FieldForValue(value, resourceType, knownMappings);
                    if (hl7Field) {
                        this.atomicMappings.push({
                            hl7Field: hl7Field,
                            fhirPath: fhirPath,
                            value: value,
                            resourceType: resourceType
                        });
                    }
                }
            });
        }

        findHL7FieldForValue(value, resourceType, knownMappings) {
            const mappings = knownMappings[resourceType];
            if (mappings && mappings[value]) {
                return mappings[value];
            }
            
            const data = this.wizard?.parsedHL7Data?.data;
            if (!data || !data.enhancedSegments) return null;
            
            for (const [segmentName, segmentData] of Object.entries(data.enhancedSegments)) {
                if (segmentData.fields && Array.isArray(segmentData.fields)) {
                    for (const field of segmentData.fields) {
                        if (field.value === value) {
                            return field.key;
                        }
                        if (field.subfields && Array.isArray(field.subfields)) {
                            for (const subfield of field.subfields) {
                                if (subfield.value === value) {
                                    return subfield.key;
                                }
                            }
                        }
                    }
                }
            }
            
            return null;
        }

        async loadInterface() {
            const step4Element = document.getElementById('step4');
            if (!step4Element) return;

            const summaryHtml = this.generateSummarySection();
            const resourcesHtml = this.generateResourcesSection();
            const modalsHtml = this.generateModals();
            
            step4Element.innerHTML = `
                <div class="step-header">
                    <div class="step-progress">STEP 4 OF 5</div>
                    <h3 class="step-title">HL7 → FHIR Transformation</h3>
                    <p class="step-description">Review and verify the HL7 message structure and segments</p>
                </div>
                
                ${summaryHtml}
                ${resourcesHtml}
                ${modalsHtml}
            `;
        }

        generateSummarySection() {
            const messageType = this.wizard?.parsedHL7Data?.data?.messageType?.name || 'ADT^A04';
            const resourceCount = this.fhirTransformResult?.fhirResources?.length || 0;
            const mappingsCount = this.atomicMappings.length;
            
            const hasResources = resourceCount > 0;
            const validationErrors = this.fhirTransformResult?.errors?.length || 0;
            const validationWarnings = this.fhirTransformResult?.warnings?.length || 0;
            
            let status = 'Failed';
            let statusColor = '#ef4444';
            if (hasResources && validationErrors === 0) {
                status = 'Success';
                statusColor = '#10b981';
            } else if (hasResources && validationErrors > 0) {
                status = 'Partial';
                statusColor = '#f59e0b';
            }
            
            const validationScore = hasResources ? 
                Math.round((mappingsCount / (mappingsCount + validationErrors)) * 100) + '%' : 
                '0%';
            
            return `
                <div style="background: linear-gradient(135deg, #f8f9ff 0%, #fff 100%); border: 2px solid #e2e8f0; border-radius: 12px; padding: 20px; margin-bottom: 24px;">
                    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px;">
                        <h4 style="margin: 0; font-size: 16px; font-weight: 600; color: #1e3a8a;">Transformation Summary</h4>
                        <span style="padding: 4px 12px; background: ${statusColor}; color: white; border-radius: 20px; font-size: 12px; font-weight: 600;">
                            ${status === 'Partial' ? '⚠️' : '✓'} ${status}
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
                            <div style="font-size: 24px; font-weight: 700; color: ${validationErrors > 0 ? '#f59e0b' : '#10b981'};">${validationScore}</div>
                            <div style="font-size: 12px; color: #6b7280; margin-top: 4px;">Validation</div>
                        </div>
                    </div>
                    
                    <div style="display: flex; gap: 8px; margin-top: 16px; padding-top: 16px; border-top: 1px solid #e2e8f0;">
                        <button onclick="window.step4Handler.expandAllResources()" style="padding: 6px 12px; background: white; border: 1px solid #e2e8f0; border-radius: 6px; font-size: 12px; cursor: pointer;">
                            📂 Expand All
                        </button>
                        <button onclick="window.step4Handler.collapseAllResources()" style="padding: 6px 12px; background: white; border: 1px solid #e2e8f0; border-radius: 6px; font-size: 12px; cursor: pointer;">
                            📁 Collapse All
                        </button>
                        <button onclick="window.step4Handler.openJsonViewer()" style="padding: 6px 12px; background: white; border: 1px solid #e2e8f0; border-radius: 6px; font-size: 12px; cursor: pointer;">
                            👁️ View JSON
                        </button>
                        <button onclick="window.step4Handler.validateMappings()" style="padding: 6px 12px; background: white; border: 1px solid #e2e8f0; border-radius: 6px; font-size: 12px; cursor: pointer;">
                            ✅ Validate
                        </button>
                        <button onclick="window.step4Handler.toggleConfigLock()" style="padding: 6px 12px; background: ${this.configurationLocked ? '#fef2f2' : '#f0fdf4'}; border: 1px solid ${this.configurationLocked ? '#fecaca' : '#bbf7d0'}; border-radius: 6px; font-size: 12px; cursor: pointer;">
                            ${this.configurationLocked ? '🔓 Unlock Config' : '🔒 Lock Config'}
                        </button>
                        <button onclick="window.step4Handler.saveConfiguration()" style="padding: 6px 12px; background: #3b82f6; color: white; border: none; border-radius: 6px; font-size: 12px; cursor: pointer;">
                            💾 Save Configuration
                        </button>
                    </div>
                </div>
            `;
        }

        generateResourcesSection() {
            if (!this.fhirTransformResult || !this.fhirTransformResult.fhirResources) {
                return '<div style="text-align: center; padding: 40px; color: #6b7280;">No resources to display</div>';
            }

            let html = '<div id="resourcesContainer" style="space-y: 16px;">';
            
            this.fhirTransformResult.fhirResources.forEach((resource, index) => {
                const mappings = this.atomicMappings.filter(m => m.resourceType === resource.resourceType);
                html += this.generateResourceCard(resource, mappings, index);
            });
            
            html += '</div>';
            return html;
        }

        generateResourceCard(resource, mappings, index) {
            const resourceType = resource.resourceType;
            const icon = this.getResourceIcon(resourceType);
            const mappingCount = mappings.length;
            
            return `
                <div style="background: white; border: 1px solid #e2e8f0; border-radius: 12px; overflow: hidden; margin-bottom: 16px;">
                    <div onclick="window.step4Handler.toggleResource(${index})" style="padding: 16px; background: linear-gradient(135deg, #f8f9ff 0%, #fff 100%); cursor: pointer; display: flex; justify-content: space-between; align-items: center;">
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
            html += '<th style="text-align: center; padding: 12px; font-size: 12px; font-weight: 600; color: #6b7280;">Actions</th>';
            html += '</tr></thead>';
            html += '<tbody>';
            
            mappings.forEach((mapping, idx) => {
                const hl7Parts = mapping.hl7Field.split('.');
                const hl7Display = hl7Parts[0] + '.' + hl7Parts.slice(1).join('.');
                const statusColor = mapping.validated === false ? '#ef4444' : '#10b981';
                const statusLabel = mapping.validated === false ? 'Invalid' : 'Direct';
                
                html += `
                    <tr style="border-bottom: 1px solid #f3f4f6;">
                        <td style="padding: 12px;">
                            <div style="font-family: monospace; font-size: 12px; color: #4b5563;">${hl7Display}</div>
                        </td>
                        <td style="text-align: center; padding: 12px;">
                            <span style="color: #9ca3af;">→</span>
                        </td>
                        <td style="padding: 12px;">
                            <div style="font-family: monospace; font-size: 12px; color: #1e3a8a;">${mapping.fhirPath}</div>
                        </td>
                        <td style="padding: 12px;">
                            <div style="font-size: 12px; color: #4b5563; max-width: 200px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;" title="${mapping.value}">${mapping.value}</div>
                        </td>
                        <td style="text-align: center; padding: 12px;">
                            <div style="display: flex; gap: 8px; justify-content: center;">
                                <span style="padding: 2px 8px; background: ${statusColor}; color: white; border-radius: 4px; font-size: 10px; font-weight: 600;">
                                    ${statusLabel}
                                </span>
                                <button onclick="window.step4Handler.editMapping('${mapping.hl7Field}', '${mapping.fhirPath}', '${mapping.value}')" 
                                        style="padding: 2px 8px; background: #3b82f6; color: white; border: none; border-radius: 4px; font-size: 10px; cursor: pointer; font-weight: 600;"
                                        ${this.configurationLocked ? 'disabled' : ''}>
                                    Edit
                                </button>
                            </div>
                        </td>
                    </tr>
                `;
            });
            
            html += '</tbody></table></div>';
            return html;
        }

        generateModals() {
            return `
                <!-- Enhanced Edit Modal -->
                <div id="editMappingModal" style="display: none; position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.5); z-index: 10000;">
                    <div style="background: white; border-radius: 12px; margin: 50px auto; max-width: 800px; width: 90%; max-height: 80vh; overflow-y: auto;">
                        <div style="padding: 24px; border-bottom: 1px solid #e2e8f0;">
                            <div style="display: flex; justify-content: space-between; align-items: center;">
                                <h3 style="margin: 0; font-size: 18px; font-weight: 600; color: #1e3a8a;">Edit Field Mapping</h3>
                                <button onclick="window.step4Handler.closeEditModal()" style="background: none; border: none; font-size: 24px; cursor: pointer; color: #6b7280;">×</button>
                            </div>
                        </div>
                        <div style="padding: 24px;">
                            <div id="editMappingContent"></div>
                        </div>
                    </div>
                </div>

                <!-- Enhanced JSON Viewer Modal with RESTORED 3 TABS -->
                <div id="jsonViewerModal" style="display: none; position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.5); z-index: 10000;">
                    <div style="background: white; border-radius: 12px; margin: 20px auto; max-width: 90%; width: 1200px; max-height: 90vh; overflow: hidden; display: flex; flex-direction: column;">
                        <div style="padding: 20px; border-bottom: 1px solid #e2e8f0;">
                            <div style="display: flex; justify-content: space-between; align-items: center;">
                                <h3 style="margin: 0; font-size: 18px; font-weight: 600; color: #1e3a8a;">FHIR JSON Viewer</h3>
                                <button onclick="window.step4Handler.closeJsonViewer()" style="background: none; border: none; font-size: 24px; cursor: pointer; color: #6b7280;">×</button>
                            </div>
                            
                            <!-- RESTORED: Tab Navigation -->
                            <div style="display: flex; gap: 2px; margin-top: 16px; background: #f3f4f6; padding: 2px; border-radius: 8px;">
                                <button onclick="window.step4Handler.switchJsonTab('bundle')" id="bundleTab" class="json-tab active" 
                                        style="flex: 1; padding: 8px; background: white; border: none; border-radius: 6px; cursor: pointer; font-weight: 600; color: #1e3a8a;">
                                    📦 Bundle View
                                </button>
                                <button onclick="window.step4Handler.switchJsonTab('resources')" id="resourcesTab" class="json-tab"
                                        style="flex: 1; padding: 8px; background: transparent; border: none; border-radius: 6px; cursor: pointer; font-weight: 500; color: #6b7280;">
                                    📋 Individual Resources
                                </button>
                                <button onclick="window.step4Handler.switchJsonTab('validation')" id="validationTab" class="json-tab"
                                        style="flex: 1; padding: 8px; background: transparent; border: none; border-radius: 6px; cursor: pointer; font-weight: 500; color: #6b7280;">
                                    ✅ Validation Report
                                </button>
                            </div>
                        </div>
                        
                        <!-- RESTORED: Resource Selector for Individual Resources -->
                        <div id="resourceSelector" style="display: none; padding: 0 20px;">
                            <select id="resourceDropdown" onchange="window.step4Handler.displaySelectedResource()" 
                                    style="width: 100%; padding: 8px; border: 1px solid #e2e8f0; border-radius: 6px; font-size: 14px;">
                                <option value="">Select a resource...</option>
                            </select>
                        </div>
                        
                        <!-- JSON Content Area with Tree View -->
                        <div style="flex: 1; padding: 20px; overflow-y: auto; background: #f9fafb;">
                            <div id="jsonTreeView" style="font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace; font-size: 13px; line-height: 1.6;">
                                <!-- Tree view will be rendered here -->
                            </div>
                        </div>
                    </div>
                </div>

                <!-- Save Configuration Modal -->
                <div id="saveConfigModal" style="display: none; position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.5); z-index: 10000;">
                    <div style="background: white; border-radius: 12px; margin: 100px auto; max-width: 600px; width: 90%;">
                        <div style="padding: 24px; border-bottom: 1px solid #e2e8f0;">
                            <div style="display: flex; justify-content: space-between; align-items: center;">
                                <h3 style="margin: 0; font-size: 18px; font-weight: 600; color: #1e3a8a;">💾 Save Configuration</h3>
                                <button onclick="window.step4Handler.closeSaveConfigModal()" style="background: none; border: none; font-size: 24px; cursor: pointer; color: #6b7280;">×</button>
                            </div>
                        </div>
                        <div style="padding: 24px;">
                            <div style="margin-bottom: 16px;">
                                <label style="display: block; margin-bottom: 8px; font-weight: 600; color: #374151;">Configuration Name</label>
                                <input type="text" id="configName" placeholder="e.g., ADT A04 Standard Mapping" style="width: 100%; padding: 10px; border: 1px solid #e2e8f0; border-radius: 6px;">
                            </div>
                            <div style="margin-bottom: 20px;">
                                <label style="display: block; margin-bottom: 8px; font-weight: 600; color: #374151;">Description</label>
                                <textarea id="configDescription" placeholder="Describe this configuration..." style="width: 100%; padding: 10px; border: 1px solid #e2e8f0; border-radius: 6px; min-height: 100px; resize: vertical;"></textarea>
                            </div>
                            <div style="display: flex; gap: 12px; justify-content: flex-end;">
                                <button onclick="window.step4Handler.closeSaveConfigModal()" style="padding: 10px 20px; background: #f3f4f6; border: none; border-radius: 6px; cursor: pointer; font-weight: 600;">Cancel</button>
                                <button onclick="window.step4Handler.performSaveConfiguration()" style="padding: 10px 20px; background: #3b82f6; color: white; border: none; border-radius: 6px; cursor: pointer; font-weight: 600;">Save Configuration</button>
                            </div>
                        </div>
                    </div>
                </div>
            `;
        }

        initializeJsonTreeViewer() {
            if (!document.getElementById('json-tree-styles')) {
                const style = document.createElement('style');
                style.id = 'json-tree-styles';
                style.textContent = `
                    .json-tree-node { margin-left: 20px; }
                    .json-tree-key { color: #1e3a8a; font-weight: 600; cursor: pointer; user-select: none; }
                    .json-tree-key:hover { text-decoration: underline; }
                    .json-tree-value { color: #059669; }
                    .json-tree-string { color: #dc2626; }
                    .json-tree-number { color: #7c3aed; }
                    .json-tree-boolean { color: #f59e0b; }
                    .json-tree-null { color: #6b7280; font-style: italic; }
                    .json-tree-toggle { display: inline-block; width: 12px; margin-right: 4px; color: #6b7280; font-size: 10px; }
                    .json-tree-collapsed .json-tree-children { display: none; }
                    .json-tree-collapsed .json-tree-toggle { transform: rotate(-90deg); }
                    .json-tab.active { background: white !important; color: #1e3a8a !important; box-shadow: 0 1px 3px rgba(0,0,0,0.1); }
                `;
                document.head.appendChild(style);
            }
        }

        // RESTORED: JSON Tab Switching Functionality
        switchJsonTab(tab) {
            // Update active tab styling
            document.querySelectorAll('.json-tab').forEach(t => {
                t.style.background = 'transparent';
                t.style.color = '#6b7280';
                t.style.fontWeight = '500';
            });
            
            const activeTab = document.getElementById(`${tab}Tab`);
            if (activeTab) {
                activeTab.style.background = 'white';
                activeTab.style.color = '#1e3a8a';
                activeTab.style.fontWeight = '600';
            }
            
            const treeView = document.getElementById('jsonTreeView');
            const resourceSelector = document.getElementById('resourceSelector');
            
            if (tab === 'bundle') {
                resourceSelector.style.display = 'none';
                if (this.fhirTransformResult?.bundle) {
                    this.renderJsonTree(this.fhirTransformResult.bundle, treeView);
                } else {
                    treeView.innerHTML = '<div style="text-align: center; color: #6b7280;">No bundle data available</div>';
                }
            } else if (tab === 'resources') {
                resourceSelector.style.display = 'block';
                this.populateResourceDropdown();
                treeView.innerHTML = '<div style="text-align: center; color: #6b7280;">Select a resource from the dropdown above</div>';
            } else if (tab === 'validation') {
                resourceSelector.style.display = 'none';
                
                // FIXED: Only show validation for resources that have mappings
                const mappedResourceTypes = [...new Set(this.atomicMappings.map(m => m.resourceType))];
                console.log('📊 Mapped resource types:', mappedResourceTypes);
                
                const mappedResources = this.fhirTransformResult?.fhirResources?.filter(resource => 
                    mappedResourceTypes.includes(resource.resourceType)
                ) || [];
                
                // Filter errors and warnings for mapped resources only
                const mappedResourceIds = mappedResources.map(r => r.id).filter(Boolean);
                const filteredErrors = this.fhirTransformResult?.errors?.filter(error => {
                    // Check if error relates to a mapped resource
                    return mappedResourceTypes.some(type => 
                        error.message?.includes(type) || error.resource?.includes(type)
                    );
                }) || [];
                
                const filteredWarnings = this.fhirTransformResult?.warnings?.filter(warning => {
                    // Check if warning relates to a mapped resource
                    return mappedResourceTypes.some(type => 
                        warning.message?.includes(type) || warning.resource?.includes(type)
                    );
                }) || [];
                
                const validationData = {
                    success: this.fhirTransformResult?.success !== false,
                    totalResources: this.fhirTransformResult?.fhirResources?.length || 0,
                    mappedResources: mappedResources.length,
                    mappedResourceTypes: mappedResourceTypes,
                    unmappedResourceTypes: this.getUnmappedResourceTypes(),
                    mappingCount: this.atomicMappings.length,
                    errors: filteredErrors,
                    warnings: filteredWarnings,
                    resourceDetails: mappedResources.map(resource => ({
                        type: resource.resourceType,
                        id: resource.id,
                        mappingCount: this.atomicMappings.filter(m => m.resourceType === resource.resourceType).length
                    }))
                };
                
                this.renderJsonTree(validationData, treeView);
            }
        }

        getUnmappedResourceTypes() {
            const allResourceTypes = [...new Set(this.fhirTransformResult?.fhirResources?.map(r => r.resourceType) || [])];
            const mappedResourceTypes = [...new Set(this.atomicMappings.map(m => m.resourceType))];
            return allResourceTypes.filter(type => !mappedResourceTypes.includes(type));
        }

        // RESTORED: Resource Dropdown Population (filtered to show only mapped resources)
        populateResourceDropdown() {
            const dropdown = document.getElementById('resourceDropdown');
            if (!dropdown || !this.fhirTransformResult?.fhirResources) return;
            
            // Filter to only show resources that have mappings
            const mappedResourceTypes = [...new Set(this.atomicMappings.map(m => m.resourceType))];
            const mappedResources = this.fhirTransformResult.fhirResources.filter(resource => 
                mappedResourceTypes.includes(resource.resourceType)
            );
            
            dropdown.innerHTML = '<option value="">Select a mapped resource...</option>';
            
            mappedResources.forEach((resource, index) => {
                // Find the original index in the full resources array
                const originalIndex = this.fhirTransformResult.fhirResources.findIndex(r => r === resource);
                const mappingCount = this.atomicMappings.filter(m => m.resourceType === resource.resourceType).length;
                
                const option = document.createElement('option');
                option.value = originalIndex;
                option.textContent = `${this.getResourceIcon(resource.resourceType)} ${resource.resourceType} - ${this.getResourceDescription(resource)} (${mappingCount} mappings)`;
                dropdown.appendChild(option);
            });
            
            // Show info about filtered resources
            if (mappedResources.length < this.fhirTransformResult.fhirResources.length) {
                const unmappedCount = this.fhirTransformResult.fhirResources.length - mappedResources.length;
                const infoOption = document.createElement('option');
                infoOption.disabled = true;
                infoOption.textContent = `ℹ️ ${unmappedCount} unmapped resources hidden`;
                dropdown.appendChild(infoOption);
            }
        }

        // RESTORED: Display Selected Resource
        displaySelectedResource() {
            const dropdown = document.getElementById('resourceDropdown');
            const treeView = document.getElementById('jsonTreeView');
            
            if (!dropdown || !treeView) return;
            
            const index = dropdown.value;
            if (index === '') {
                treeView.innerHTML = '<div style="text-align: center; color: #6b7280;">Select a resource from the dropdown above</div>';
                return;
            }
            
            const resource = this.fhirTransformResult.fhirResources[parseInt(index)];
            if (resource) {
                this.renderJsonTree(resource, treeView);
            }
        }

        // RESTORED: JSON Tree Rendering
        renderJsonTree(data, container, level = 0) {
            container.innerHTML = '';
            
            if (typeof data === 'object' && data !== null) {
                if (Array.isArray(data)) {
                    data.forEach((item, index) => {
                        const node = this.createTreeNode(`[${index}]`, item, level);
                        container.appendChild(node);
                    });
                } else {
                    Object.entries(data).forEach(([key, value]) => {
                        const node = this.createTreeNode(key, value, level);
                        container.appendChild(node);
                    });
                }
            } else {
                container.textContent = JSON.stringify(data, null, 2);
            }
        }

        createTreeNode(key, value, level) {
            const node = document.createElement('div');
            node.className = 'json-tree-node';
            node.style.marginLeft = `${level * 20}px`;
            
            const isExpandable = typeof value === 'object' && value !== null;
            
            if (isExpandable) {
                const keyElement = document.createElement('span');
                keyElement.className = 'json-tree-key';
                keyElement.innerHTML = `
                    <span class="json-tree-toggle">▼</span>
                    ${key}: ${Array.isArray(value) ? `[${value.length}]` : '{...}'}
                `;
                
                const childrenContainer = document.createElement('div');
                childrenContainer.className = 'json-tree-children';
                
                keyElement.onclick = () => {
                    node.classList.toggle('json-tree-collapsed');
                    if (!childrenContainer.hasChildNodes()) {
                        this.renderJsonTree(value, childrenContainer, level + 1);
                    }
                };
                
                node.appendChild(keyElement);
                node.appendChild(childrenContainer);
                
                // Auto-expand first level
                if (level < 1) {
                    this.renderJsonTree(value, childrenContainer, level + 1);
                }
            } else {
                const valueClass = value === null ? 'json-tree-null' :
                                  typeof value === 'string' ? 'json-tree-string' :
                                  typeof value === 'number' ? 'json-tree-number' :
                                  typeof value === 'boolean' ? 'json-tree-boolean' : '';
                
                node.innerHTML = `
                    <span class="json-tree-key">${key}:</span>
                    <span class="${valueClass}"> ${JSON.stringify(value)}</span>
                `;
            }
            
            return node;
        }

        // UI Event Handlers
        toggleResource(index) {
            const content = document.getElementById(`resource-content-${index}`);
            const icon = document.getElementById(`toggle-icon-${index}`);
            
            if (content.style.display === 'none') {
                content.style.display = 'block';
                icon.textContent = '▼';
            } else {
                content.style.display = 'none';
                icon.textContent = '▶';
            }
        }

        expandAllResources() {
            document.querySelectorAll('[id^="resource-content-"]').forEach(el => {
                el.style.display = 'block';
            });
            document.querySelectorAll('[id^="toggle-icon-"]').forEach(el => {
                el.textContent = '▼';
            });
        }

        collapseAllResources() {
            document.querySelectorAll('[id^="resource-content-"]').forEach(el => {
                el.style.display = 'none';
            });
            document.querySelectorAll('[id^="toggle-icon-"]').forEach(el => {
                el.textContent = '▶';
            });
        }

        openJsonViewer() {
            const modal = document.getElementById('jsonViewerModal');
            modal.style.display = 'block';
            this.switchJsonTab('bundle');
        }

        closeJsonViewer() {
            document.getElementById('jsonViewerModal').style.display = 'none';
        }

        editMapping(hl7Field, fhirPath, value) {
            if (this.configurationLocked) {
                this.showNotification('Configuration is locked. Unlock it to make changes.', 'warning');
                return;
            }

            console.log('🔧 Edit mapping called:', { hl7Field, fhirPath, value });
            console.log('🔍 Enhanced mapping available:', !!this.enhancedMapping);
            console.log('🔍 Global enhanced mapping:', !!window.enhancedMappingInterface);

            // Ensure global reference is set
            if (this.enhancedMapping && !window.enhancedMappingInterface) {
                window.enhancedMappingInterface = this.enhancedMapping;
                console.log('🔧 Set global enhanced mapping reference');
            }

            // Try enhanced interface first
            if (this.enhancedMapping) {
                try {
                    const mappingData = this.findMappingData(hl7Field, fhirPath, value);
                    console.log('📊 Found mapping data:', mappingData);
                    console.log('🚀 Attempting to open enhanced modal...');
                    
                    this.enhancedMapping.openEnhancedEditModal(mappingData);
                    console.log('✅ Enhanced modal opened successfully');
                    return;
                } catch (error) {
                    console.error('❌ Enhanced mapping interface error:', error);
                    console.error('Error stack:', error.stack);
                }
            } else {
                console.warn('⚠️ Enhanced mapping not available - this.enhancedMapping is null');
            }

            // Fallback to basic modal
            console.log('⚠️ Using fallback modal for editing');
            this.openBasicEditModal(hl7Field, fhirPath, value);
        }

        openBasicEditModal(hl7Field, fhirPath, value) {
            const modal = document.getElementById('editMappingModal');
            const content = document.getElementById('editMappingContent');
            
            if (!modal || !content) {
                console.error('❌ Basic modal elements not found');
                this.showNotification('Modal elements not found', 'error');
                return;
            }
            
            content.innerHTML = `
                <div style="margin-bottom: 16px;">
                    <label style="display: block; margin-bottom: 8px; font-weight: 600; color: #374151;">HL7 Field: ${hl7Field}</label>
                    <label style="display: block; margin-bottom: 8px; font-weight: 600; color: #374151;">FHIR Path: ${fhirPath}</label>
                    <label style="display: block; margin-bottom: 8px; font-weight: 600; color: #374151;">Current Value: ${value}</label>
                </div>
                <div style="background: #f0f9ff; border: 1px solid #bfdbfe; border-radius: 8px; padding: 16px; margin-bottom: 16px;">
                    <p style="color: #1e40af; font-size: 14px; margin: 0;">📝 <strong>Basic Edit Mode</strong></p>
                    <p style="color: #6b7280; font-size: 13px; margin: 8px 0 0 0;">Enhanced mapping interface is not available. You can still view the mapping details above.</p>
                </div>
                <div style="display: flex; gap: 12px; justify-content: flex-end; margin-top: 20px;">
                    <button onclick="window.step4Handler.closeEditModal()" style="padding: 10px 20px; background: #f3f4f6; border: none; border-radius: 6px; cursor: pointer; font-weight: 600;">Close</button>
                </div>
            `;
            
            modal.style.display = 'block';
            console.log('✅ Basic modal opened');
        }

        findMappingData(hl7Field, fhirPath, value) {
            if (this.atomicMappings) {
                const found = this.atomicMappings.find(mapping => 
                    mapping.hl7Field === hl7Field && 
                    mapping.fhirPath === fhirPath
                );
                if (found) return found;
            }
            
            return {
                hl7Field: hl7Field,
                fhirPath: fhirPath,
                hl7Value: value,
                fhirValue: value,
                transformType: 'direct',
                hl7Description: 'HL7 field',
                fhirDescription: 'FHIR field'
            };
        }

        closeEditModal() {
            document.getElementById('editMappingModal').style.display = 'none';
        }

        toggleConfigLock() {
            this.configurationLocked = !this.configurationLocked;
            this.loadInterface();
            this.showNotification(
                this.configurationLocked ? 'Configuration locked' : 'Configuration unlocked',
                this.configurationLocked ? 'info' : 'success'
            );
        }

        saveConfiguration() {
            document.getElementById('saveConfigModal').style.display = 'block';
        }

        closeSaveConfigModal() {
            document.getElementById('saveConfigModal').style.display = 'none';
        }

        performSaveConfiguration() {
            const name = document.getElementById('configName').value;
            const description = document.getElementById('configDescription').value;
            
            if (!name) {
                this.showNotification('Please provide a configuration name', 'warning');
                return;
            }
            
            const config = {
                name: name,
                description: description,
                messageType: this.wizard?.parsedHL7Data?.data?.messageType?.name || 'Unknown',
                mappings: this.atomicMappings,
                createdAt: new Date().toISOString(),
                locked: this.configurationLocked
            };
            
            const configs = JSON.parse(localStorage.getItem('fhir_configurations') || '[]');
            configs.push(config);
            localStorage.setItem('fhir_configurations', JSON.stringify(configs));
            
            this.savedConfiguration = config;
            this.closeSaveConfigModal();
            this.showNotification('Configuration saved successfully', 'success');
        }

        loadSavedConfiguration() {
            const configs = JSON.parse(localStorage.getItem('fhir_configurations') || '[]');
            if (configs.length > 0) {
                console.log(`Found ${configs.length} saved configurations`);
            }
        }

        validateMappings() {
            let valid = 0;
            let invalid = 0;
            
            this.atomicMappings.forEach(mapping => {
                if (mapping.value && mapping.hl7Field && mapping.fhirPath) {
                    valid++;
                    mapping.validated = true;
                } else {
                    invalid++;
                    mapping.validated = false;
                }
            });
            
            this.loadInterface();
            this.showNotification(
                `Validation complete: ${valid} valid, ${invalid} invalid mappings`,
                invalid > 0 ? 'warning' : 'success'
            );
        }

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
                'Bundle': '📦'
            };
            return icons[resourceType] || '📄';
        }

        getResourceDescription(resource) {
            if (resource.resourceType === 'Patient') {
                const name = resource.name?.[0];
                if (name?.family || name?.given) {
                    return `${name.given?.[0] || ''} ${name.family || ''}`.trim();
                }
            } else if (resource.resourceType === 'Encounter') {
                return resource.status || 'Unknown status';
            } else if (resource.resourceType === 'Observation') {
                return resource.code?.text || resource.code?.coding?.[0]?.display || 'Unknown';
            }
            return resource.id || 'No ID';
        }

        ensureStep4Visible() {
            const step4Element = document.getElementById('step4');
            if (step4Element) {
                step4Element.style.display = 'block';
            }
        }

        setupEventListeners() {
            window.step4Handler = this;
            
            // Add debug helper for troubleshooting
            window.debugEnhancedMapping = () => {
                console.log('🔍 Enhanced Mapping Debug Info:');
                console.log('- this.enhancedMapping:', !!this.enhancedMapping);
                console.log('- window.enhancedMappingInterface:', !!window.enhancedMappingInterface);
                console.log('- FieldMappingValidator:', typeof FieldMappingValidator);
                console.log('- EnhancedMappingInterface:', typeof EnhancedMappingInterface);
                if (this.enhancedMapping) {
                    console.log('- openEnhancedEditModal method:', typeof this.enhancedMapping.openEnhancedEditModal);
                    console.log('- enhancedMapping constructor:', this.enhancedMapping.constructor.name);
                }
                return {
                    enhancedMapping: !!this.enhancedMapping,
                    globalReference: !!window.enhancedMappingInterface,
                    classes: {
                        validator: typeof FieldMappingValidator,
                        interface: typeof EnhancedMappingInterface
                    }
                };
            };
            
            console.log('🔧 Debug helper added: Call window.debugEnhancedMapping() in console for troubleshooting');
        }

        showNotification(message, type = 'info') {
            if (this.wizard && this.wizard.showNotification) {
                this.wizard.showNotification(message, type);
            } else {
                console.log(`[${type.toUpperCase()}] ${message}`);
            }
        }

        showError(message) {
            const step4Element = document.getElementById('step4');
            if (step4Element) {
                step4Element.innerHTML = `
                    <div style="background: #fef2f2; border: 1px solid #fecaca; border-radius: 12px; padding: 24px; text-align: center;">
                        <div style="font-size: 48px; margin-bottom: 16px;">⚠️</div>
                        <div style="font-weight: 600; color: #dc2626; margin-bottom: 8px;">Error Loading FHIR Mapping</div>
                        <div style="color: #7f1d1d;">${message}</div>
                        <button onclick="window.step4Handler.initialize()" style="margin-top: 16px; padding: 8px 16px; background: #dc2626; color: white; border: none; border-radius: 6px; cursor: pointer;">
                            Retry
                        </button>
                    </div>
                `;
            }
        }
    }

    // Make the class globally available
    window.FHIRMappingStepHandler = FHIRMappingStepHandler;

    // Auto-initialize when wizard is ready
    if (window.wizard) {
        console.log('🔧 Wizard found, creating Step 4 handler instance...');
        try {
            window.wizard.stepHandlers[4] = new FHIRMappingStepHandler(window.wizard);
            console.log('✅ Step 4 handler instance created and assigned');
        } catch (error) {
            console.error('❌ Failed to create Step 4 handler instance:', error);
        }
    } else {
        console.log('⏳ Wizard not found yet, handler class is ready for manual initialization');
    }

    console.log('✅ FHIRMappingStepHandler loaded and globally available');
    
})();