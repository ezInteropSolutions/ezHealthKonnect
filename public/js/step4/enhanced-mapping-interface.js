// enhanced-mapping-interface.js - Clean version without duplicates
// Enhanced Source-to-Destination Mapping Interface
// Provides comprehensive mapping editing with validation and suggestions

(function() {
    'use strict';
    
    // Prevent duplicate declarations
    if (typeof window.EnhancedMappingInterface !== 'undefined') {
        console.log('ℹ️ EnhancedMappingInterface already loaded, skipping redeclaration');
        return;
    }

    /**
     * Enhanced Source-to-Destination Mapping Interface
     * Provides comprehensive mapping editing with validation and suggestions
     */
    class EnhancedMappingInterface {
        constructor(handler) {
            this.handler = handler;
            this.validator = null;
            this.currentMapping = null;
            this.availableHL7Fields = new Map();
            this.availableFHIRFields = new Map();
            this.pendingChanges = new Map();
            
            this.initializeInterface();
        }

        /**
         * Initialize the enhanced mapping interface
         */
        initializeInterface() {
            // Wait for validator to be available
            this.waitForValidator().then(() => {
                this.loadAvailableFields();
                this.bindEventListeners();
            });
        }

        async waitForValidator() {
            let attempts = 0;
            while (attempts < 10 && typeof window.FieldMappingValidator === 'undefined') {
                await new Promise(resolve => setTimeout(resolve, 100));
                attempts++;
            }
            
            if (typeof window.FieldMappingValidator !== 'undefined') {
                this.validator = new window.FieldMappingValidator();
                console.log('✅ FieldMappingValidator initialized in EnhancedMappingInterface');
            } else {
                console.warn('⚠️ FieldMappingValidator not available, validation disabled');
            }
        }

        /**
         * Open the enhanced edit modal with validation and suggestions
         * @param {Object} mappingData - Current mapping data
         */
        openEnhancedEditModal(mappingData) {
            try {
                console.log('📝 openEnhancedEditModal called with:', mappingData);
                
                this.currentMapping = mappingData;
                this.buildEnhancedModal();
                this.populateCurrentMapping();
                this.validateCurrentMapping();
                this.showModal();
                
                console.log('✅ Enhanced modal should now be visible');
            } catch (error) {
                console.error('❌ Error in openEnhancedEditModal:', error);
                console.error('Stack:', error.stack);
                throw error;
            }
        }

        /**
         * Test method to verify the interface is working
         */
        testInterface() {
            console.log('🧪 Testing Enhanced Mapping Interface');
            console.log('- Handler:', !!this.handler);
            console.log('- Validator:', !!this.validator);
            console.log('- Available methods:', Object.getOwnPropertyNames(Object.getPrototypeOf(this)));
            
            // Test basic modal creation
            try {
                this.buildEnhancedModal();
                console.log('✅ Modal HTML creation test passed');
            } catch (error) {
                console.error('❌ Modal HTML creation test failed:', error);
            }
            
            return 'Interface test complete - check console for results';
        }

        /**
         * Build the enhanced modal HTML structure
         */
        buildEnhancedModal() {
            console.log('🏗️ Building enhanced modal HTML...');
            
            // Remove existing modal if present
            const existingModal = document.getElementById('enhancedMappingOverlay');
            if (existingModal) {
                console.log('🗑️ Removing existing modal');
                existingModal.remove();
            }

            const modalHTML = `
                <div id="enhancedMappingOverlay" class="mapping-overlay" style="position: fixed; top: 0; left: 0; width: 100%; height: 100%; background: rgba(0, 0, 0, 0.5); display: flex; justify-content: center; align-items: center; z-index: 10000; opacity: 0; visibility: hidden; transition: opacity 0.3s ease, visibility 0.3s ease;">
                    <div class="mapping-modal enhanced" style="background: white; border-radius: 12px; width: 90%; max-width: 900px; max-height: 90vh; overflow-y: auto; box-shadow: 0 20px 40px rgba(0, 0, 0, 0.2); transform: translateY(-20px); transition: transform 0.3s ease;">
                        <div class="modal-header" style="display: flex; justify-content: space-between; align-items: center; padding: 20px 24px; border-bottom: 1px solid #e5e7eb; background: #f8fafc; border-radius: 12px 12px 0 0;">
                            <h3 style="margin: 0; font-size: 18px; font-weight: 600; color: #1f2937;">Edit Source → Destination Mapping</h3>
                            <button class="close-btn" onclick="window.enhancedMappingInterface.closeEnhancedModal()" style="background: none; border: none; font-size: 24px; cursor: pointer; color: #6b7280; padding: 4px; border-radius: 4px; transition: color 0.2s ease;">×</button>
                        </div>
                        
                        <div class="modal-content" style="padding: 24px;">
                            <!-- Current Mapping Display -->
                            <div class="current-mapping-section" style="margin-bottom: 32px; padding-bottom: 24px; border-bottom: 1px solid #f3f4f6;">
                                <h4 style="margin: 0 0 16px 0; font-size: 16px; font-weight: 600; color: #374151;">Current Mapping</h4>
                                <div class="mapping-display" style="display: flex; align-items: center; gap: 20px; padding: 16px; background: #f8fafc; border-radius: 8px; border: 1px solid #e5e7eb;">
                                    <div class="source-field" style="flex: 1;">
                                        <label style="display: block; font-size: 12px; font-weight: 600; color: #6b7280; margin-bottom: 8px; text-transform: uppercase; letter-spacing: 0.5px;">HL7 Source</label>
                                        <div class="field-info" style="display: flex; flex-direction: column; gap: 4px;">
                                            <span id="currentHL7Path" class="path" style="font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace; font-size: 13px; font-weight: 600; color: #1f2937;"></span>
                                            <span id="currentHL7Value" class="value" style="font-size: 12px; color: #6b7280; font-style: italic; word-break: break-all;"></span>
                                        </div>
                                    </div>
                                    <div class="arrow" style="font-size: 18px; color: #3b82f6; font-weight: bold; margin: 0 8px;">→</div>
                                    <div class="destination-field" style="flex: 1;">
                                        <label style="display: block; font-size: 12px; font-weight: 600; color: #6b7280; margin-bottom: 8px; text-transform: uppercase; letter-spacing: 0.5px;">FHIR Destination</label>
                                        <div class="field-info" style="display: flex; flex-direction: column; gap: 4px;">
                                            <span id="currentFHIRPath" class="path" style="font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace; font-size: 13px; font-weight: 600; color: #1f2937;"></span>
                                            <span id="currentFHIRValue" class="value" style="font-size: 12px; color: #6b7280; font-style: italic; word-break: break-all;"></span>
                                        </div>
                                    </div>
                                </div>
                            </div>

                            <!-- New Mapping Configuration -->
                            <div class="new-mapping-section" style="margin-bottom: 32px; padding-bottom: 24px; border-bottom: 1px solid #f3f4f6;">
                                <h4 style="margin: 0 0 16px 0; font-size: 16px; font-weight: 600; color: #374151;">Configure New Mapping</h4>
                                <div class="mapping-config" style="display: grid; grid-template-columns: 1fr 1fr; gap: 24px;">
                                    <div class="field-selector" style="display: flex; flex-direction: column;">
                                        <label for="newHL7Source" style="font-size: 14px; font-weight: 600; color: #374151; margin-bottom: 8px;">Select HL7 Source Field</label>
                                        <select id="newHL7Source" class="field-dropdown" style="width: 100%; padding: 12px; border: 1px solid #d1d5db; border-radius: 6px; font-size: 14px; background: white;">
                                            <option value="">Choose HL7 field...</option>
                                        </select>
                                        <div id="hl7FieldDetails" class="field-details" style="margin-top: 8px; padding: 8px; background: #f8fafc; border-radius: 4px; font-size: 12px; color: #6b7280; min-height: 20px;"></div>
                                    </div>

                                    <div class="field-selector" style="display: flex; flex-direction: column;">
                                        <label for="newFHIRDestination" style="font-size: 14px; font-weight: 600; color: #374151; margin-bottom: 8px;">Select FHIR Destination Field</label>
                                        <select id="newFHIRDestination" class="field-dropdown" style="width: 100%; padding: 12px; border: 1px solid #d1d5db; border-radius: 6px; font-size: 14px; background: white;">
                                            <option value="">Choose FHIR field...</option>
                                        </select>
                                        <div id="fhirFieldDetails" class="field-details" style="margin-top: 8px; padding: 8px; background: #f8fafc; border-radius: 4px; font-size: 12px; color: #6b7280; min-height: 20px;"></div>
                                    </div>
                                </div>
                            </div>

                            <!-- Validation Results -->
                            <div id="validationSection" class="validation-section" style="margin-bottom: 32px; padding-bottom: 24px; border-bottom: 1px solid #f3f4f6;">
                                <h4 style="margin: 0 0 16px 0; font-size: 16px; font-weight: 600; color: #374151;">Mapping Validation</h4>
                                <div id="validationResults" class="validation-results" style="padding: 16px; border-radius: 8px; border: 1px solid #e5e7eb;">
                                    <div class="validation-neutral" style="color: #6b7280; font-style: italic;">Select fields to validate mapping</div>
                                </div>
                            </div>

                            <!-- Transform Configuration -->
                            <div class="transform-section" style="margin-bottom: 32px; padding-bottom: 24px; border-bottom: 1px solid #f3f4f6;">
                                <h4 style="margin: 0 0 16px 0; font-size: 16px; font-weight: 600; color: #374151;">Transformation Rules</h4>
                                <div class="transform-config" style="display: flex; flex-direction: column; gap: 16px;">
                                    <div class="transform-type">
                                        <label style="display: block; font-size: 14px; font-weight: 600; color: #374151; margin-bottom: 8px;">Transform Type</label>
                                        <select id="transformType" class="transform-dropdown" style="width: 100%; padding: 12px; border: 1px solid #d1d5db; border-radius: 6px; font-size: 14px; background: white;">
                                            <option value="direct">Direct Mapping</option>
                                            <option value="concatenate">Concatenate Fields</option>
                                            <option value="split">Split Field</option>
                                            <option value="lookup">Code Lookup</option>
                                            <option value="dateConvert">Date Conversion</option>
                                            <option value="custom">Custom Transform</option>
                                        </select>
                                    </div>
                                    <div id="transformRules" class="transform-rules" style="padding: 16px; background: #f8fafc; border-radius: 8px; border: 1px solid #e5e7eb; min-height: 60px;"></div>
                                </div>
                            </div>

                            <!-- Preview Section -->
                            <div class="preview-section" style="margin-bottom: 0; padding-bottom: 0;">
                                <h4 style="margin: 0 0 16px 0; font-size: 16px; font-weight: 600; color: #374151;">Transformation Preview</h4>
                                <div id="transformPreview" class="transform-preview" style="padding: 16px; background: #f0f9ff; border-radius: 8px; border: 1px solid #bfdbfe;">
                                    <div class="preview-input" style="display: flex; align-items: center; gap: 12px; margin-bottom: 12px;">
                                        <label style="font-size: 13px; font-weight: 600; color: #374151; min-width: 100px;">Input Value:</label>
                                        <span id="previewInput" style="font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace; font-size: 13px; padding: 6px 8px; background: white; border-radius: 4px; border: 1px solid #d1d5db; word-break: break-all; flex: 1;">No input selected</span>
                                    </div>
                                    <div class="preview-arrow" style="text-align: center; font-size: 16px; color: #3b82f6; font-weight: bold; margin: 8px 0;">↓</div>
                                    <div class="preview-output" style="display: flex; align-items: center; gap: 12px; margin-bottom: 0;">
                                        <label style="font-size: 13px; font-weight: 600; color: #374151; min-width: 100px;">Expected Output:</label>
                                        <span id="previewOutput" style="font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace; font-size: 13px; padding: 6px 8px; background: white; border-radius: 4px; border: 1px solid #d1d5db; word-break: break-all; flex: 1;">No output available</span>
                                    </div>
                                </div>
                            </div>
                        </div>

                        <div class="modal-actions" style="display: flex; justify-content: flex-end; gap: 12px; padding: 20px 24px; border-top: 1px solid #e5e7eb; background: #f8fafc; border-radius: 0 0 12px 12px;">
                            <button class="btn-secondary" onclick="window.enhancedMappingInterface.closeEnhancedModal()" style="padding: 12px 24px; border: none; border-radius: 6px; font-size: 14px; font-weight: 600; cursor: pointer; min-width: 100px; background: #f3f4f6; color: #374151; border: 1px solid #d1d5db;">Cancel</button>
                            <button class="btn-primary" onclick="window.enhancedMappingInterface.saveEnhancedMapping()" id="saveButton" style="padding: 12px 24px; border: none; border-radius: 6px; font-size: 14px; font-weight: 600; cursor: pointer; min-width: 100px; background: #3b82f6; color: white;">Save Mapping</button>
                        </div>
                    </div>
                </div>
            `;

            // Add new modal to DOM
            document.body.insertAdjacentHTML('beforeend', modalHTML);
            
            // Verify modal was created
            const newModal = document.getElementById('enhancedMappingOverlay');
            if (newModal) {
                console.log('✅ Enhanced modal HTML created successfully');
                console.log('📏 Modal element:', {
                    exists: !!newModal,
                    display: newModal.style.display,
                    zIndex: newModal.style.zIndex,
                    position: newModal.style.position
                });
            } else {
                console.error('❌ Failed to create enhanced modal HTML');
            }
        }

        /**
         * Populate the current mapping information in the modal
         */
        populateCurrentMapping() {
            if (!this.currentMapping) return;

            const currentHL7Path = document.getElementById('currentHL7Path');
            const currentHL7Value = document.getElementById('currentHL7Value');
            const currentFHIRPath = document.getElementById('currentFHIRPath');
            const currentFHIRValue = document.getElementById('currentFHIRValue');

            if (currentHL7Path) currentHL7Path.textContent = this.currentMapping.hl7Field || 'N/A';
            if (currentHL7Value) currentHL7Value.textContent = `"${this.currentMapping.hl7Value || ''}"`;
            if (currentFHIRPath) currentFHIRPath.textContent = this.currentMapping.fhirPath || 'N/A';
            if (currentFHIRValue) currentFHIRValue.textContent = `"${this.currentMapping.fhirValue || ''}"`;

            // Populate dropdowns
            this.populateHL7SourceDropdown();
            this.populateFHIRDestinationDropdown();

            // Set current selections
            const hl7Select = document.getElementById('newHL7Source');
            const fhirSelect = document.getElementById('newFHIRDestination');
            const transformSelect = document.getElementById('transformType');

            if (hl7Select) hl7Select.value = this.currentMapping.hl7Field || '';
            if (fhirSelect) fhirSelect.value = this.currentMapping.fhirPath || '';
            if (transformSelect) transformSelect.value = this.currentMapping.transformType || 'direct';
        }

        /**
         * Populate HL7 source field dropdown with enhanced information
         */
        populateHL7SourceDropdown() {
            const dropdown = document.getElementById('newHL7Source');
            if (!dropdown) return;

            const parsedData = this.handler.wizard?.parsedHL7Data?.data;
            if (!parsedData?.enhancedSegments) return;

            let options = '<option value="">Choose HL7 field...</option>';
            
            Object.entries(parsedData.enhancedSegments).forEach(([segmentName, segment]) => {
                if (segment.fields && Object.keys(segment.fields).length > 0) {
                    options += `<optgroup label="${segmentName} Segment">`;
                    
                    Object.entries(segment.fields).forEach(([fieldId, field]) => {
                        const fieldPath = `${segmentName}.${fieldId}`;
                        const value = field.value || '';
                        const description = field.description || field.name || 'No description';
                        const dataType = field.dataType || 'Unknown';
                        
                        options += `<option value="${fieldPath}" 
                                          data-type="${dataType}" 
                                          data-value="${value}" 
                                          data-description="${description}">
                                    ${fieldPath} - ${description} (${dataType})
                                    </option>`;
                    });
                    options += '</optgroup>';
                }
            });

            dropdown.innerHTML = options;
        }

        /**
         * Populate FHIR destination field dropdown with resource grouping
         */
        populateFHIRDestinationDropdown() {
            const dropdown = document.getElementById('newFHIRDestination');
            if (!dropdown) return;

            const fhirFieldsByResource = {
                'Patient': [
                    { path: 'Patient.identifier[0].value', type: 'string', desc: 'Primary identifier (MRN)' },
                    { path: 'Patient.identifier[1].value', type: 'string', desc: 'Secondary identifier' },
                    { path: 'Patient.name[0].family', type: 'string', desc: 'Family name' },
                    { path: 'Patient.name[0].given[0]', type: 'string', desc: 'First given name' },
                    { path: 'Patient.name[0].given[1]', type: 'string', desc: 'Middle name' },
                    { path: 'Patient.gender', type: 'code', desc: 'Administrative gender' },
                    { path: 'Patient.birthDate', type: 'date', desc: 'Date of birth' },
                    { path: 'Patient.telecom[0].value', type: 'string', desc: 'Primary phone' },
                    { path: 'Patient.address[0].line[0]', type: 'string', desc: 'Street address' },
                    { path: 'Patient.address[0].city', type: 'string', desc: 'City' },
                    { path: 'Patient.address[0].state', type: 'string', desc: 'State/Province' },
                    { path: 'Patient.address[0].postalCode', type: 'string', desc: 'ZIP/Postal code' }
                ],
                'MessageHeader': [
                    { path: 'MessageHeader.source.name', type: 'string', desc: 'Sending application' },
                    { path: 'MessageHeader.source.endpoint', type: 'uri', desc: 'Source endpoint' },
                    { path: 'MessageHeader.destination[0].name', type: 'string', desc: 'Receiving application' },
                    { path: 'MessageHeader.destination[0].endpoint', type: 'uri', desc: 'Destination endpoint' }
                ],
                'Encounter': [
                    { path: 'Encounter.status', type: 'code', desc: 'Status code' },
                    { path: 'Encounter.class.code', type: 'code', desc: 'Encounter class' },
                    { path: 'Encounter.subject.reference', type: 'Reference', desc: 'Patient reference' },
                    { path: 'Encounter.period.start', type: 'dateTime', desc: 'Start time' },
                    { path: 'Encounter.period.end', type: 'dateTime', desc: 'End time' }
                ]
            };

            let options = '<option value="">Choose FHIR field...</option>';
            
            Object.entries(fhirFieldsByResource).forEach(([resourceType, fields]) => {
                options += `<optgroup label="${resourceType} Resource">`;
                fields.forEach(field => {
                    options += `<option value="${field.path}" 
                                      data-type="${field.type}" 
                                      data-description="${field.desc}">
                                ${field.path} - ${field.desc} (${field.type})
                                </option>`;
                });
                options += '</optgroup>';
            });

            dropdown.innerHTML = options;
        }

        /**
         * Validate the current mapping configuration
         */
        validateCurrentMapping() {
            const hl7Field = this.getSelectedHL7Field();
            const fhirField = this.getSelectedFHIRField();

            if (!hl7Field || !fhirField) {
                this.clearValidationResults();
                return;
            }

            if (this.validator) {
                const validation = this.validator.validateMapping(hl7Field, fhirField);
                this.displayValidationResults(validation);
                
                if (validation.suggestedTransform) {
                    const transformSelect = document.getElementById('transformType');
                    if (transformSelect) {
                        transformSelect.value = validation.suggestedTransform;
                    }
                }
            } else {
                this.displayValidationResults({
                    valid: true,
                    warnings: ['Validation service not available'],
                    errors: [],
                    suggestions: []
                });
            }

            this.updateTransformPreview();
        }

        /**
         * Display validation results in the UI
         */
        displayValidationResults(validation) {
            const resultsContainer = document.getElementById('validationResults');
            if (!resultsContainer) return;

            let html = '';

            if (validation.valid) {
                html += '<div class="validation-success">✅ Mapping is valid</div>';
            } else {
                html += '<div class="validation-error">❌ Mapping has issues</div>';
            }

            if (validation.errors && validation.errors.length > 0) {
                html += '<div class="validation-errors">';
                html += '<h5>Errors:</h5>';
                validation.errors.forEach(error => {
                    html += `<div class="error-item">• ${error}</div>`;
                });
                html += '</div>';
            }

            if (validation.warnings && validation.warnings.length > 0) {
                html += '<div class="validation-warnings">';
                html += '<h5>Warnings:</h5>';
                validation.warnings.forEach(warning => {
                    html += `<div class="warning-item">• ${warning}</div>`;
                });
                html += '</div>';
            }

            if (validation.suggestions && validation.suggestions.length > 0) {
                html += '<div class="validation-suggestions">';
                html += '<h5>Suggestions:</h5>';
                validation.suggestions.forEach(suggestion => {
                    html += `<div class="suggestion-item">💡 ${suggestion}</div>`;
                });
                html += '</div>';
            }

            resultsContainer.innerHTML = html;
        }

        /**
         * Update the transformation preview based on current selection
         */
        updateTransformPreview() {
            const hl7Field = this.getSelectedHL7Field();
            const transformType = document.getElementById('transformType')?.value || 'direct';
            
            const previewInput = document.getElementById('previewInput');
            const previewOutput = document.getElementById('previewOutput');

            if (!hl7Field || !previewInput || !previewOutput) {
                if (previewInput) previewInput.textContent = 'No input selected';
                if (previewOutput) previewOutput.textContent = 'No output available';
                return;
            }

            const inputValue = hl7Field.value || 'Sample value';
            previewInput.textContent = inputValue;

            const previewOutputValue = this.generateTransformPreview(inputValue, transformType);
            previewOutput.textContent = previewOutputValue;
        }

        /**
         * Generate transformation preview output
         */
        generateTransformPreview(inputValue, transformType) {
            switch (transformType) {
                case 'direct':
                    return inputValue;
                case 'concatenate':
                    return `${inputValue} + [additional field]`;
                case 'split':
                    return inputValue.split(' ')[0] || inputValue;
                case 'lookup':
                    return `Mapped code for: ${inputValue}`;
                case 'dateConvert':
                    return this.convertDateFormat(inputValue);
                case 'custom':
                    return `Custom transform of: ${inputValue}`;
                default:
                    return inputValue;
            }
        }

        convertDateFormat(dateValue) {
            if (/^\d{8}$/.test(dateValue)) {
                return `${dateValue.substring(0,4)}-${dateValue.substring(4,6)}-${dateValue.substring(6,8)}`;
            }
            return dateValue;
        }

        /**
         * Get selected HL7 field information
         */
        getSelectedHL7Field() {
            const dropdown = document.getElementById('newHL7Source');
            if (!dropdown) return null;
            
            const selectedOption = dropdown.options[dropdown.selectedIndex];
            
            if (!selectedOption || !selectedOption.value) return null;

            return {
                path: selectedOption.value,
                type: selectedOption.dataset.type,
                value: selectedOption.dataset.value,
                description: selectedOption.dataset.description
            };
        }

        /**
         * Get selected FHIR field information
         */
        getSelectedFHIRField() {
            const dropdown = document.getElementById('newFHIRDestination');
            if (!dropdown) return null;
            
            const selectedOption = dropdown.options[dropdown.selectedIndex];
            
            if (!selectedOption || !selectedOption.value) return null;

            return {
                path: selectedOption.value,
                type: selectedOption.dataset.type,
                description: selectedOption.dataset.description
            };
        }

        /**
         * Bind event listeners for the interface
         */
        bindEventListeners() {
            document.addEventListener('change', (e) => {
                if (e.target.id === 'newHL7Source' || e.target.id === 'newFHIRDestination') {
                    this.validateCurrentMapping();
                }
                if (e.target.id === 'transformType') {
                    this.updateTransformPreview();
                }
            });
        }

        /**
         * Save the enhanced mapping configuration
         */
        async saveEnhancedMapping() {
            const hl7Field = this.getSelectedHL7Field();
            const fhirField = this.getSelectedFHIRField();
            const transformType = document.getElementById('transformType')?.value || 'direct';

            if (!hl7Field || !fhirField) {
                this.handler.showNotification('Please select both source and destination fields', 'warning');
                return;
            }

            if (this.validator) {
                const validation = this.validator.validateMapping(hl7Field, fhirField);
                if (!validation.valid && validation.errors && validation.errors.length > 0) {
                    this.handler.showNotification('Cannot save mapping with validation errors', 'error');
                    return;
                }
            }

            const mappingChange = {
                originalHL7Field: this.currentMapping.hl7Field,
                originalFHIRField: this.currentMapping.fhirPath,
                newHL7Field: hl7Field.path,
                newFHIRField: fhirField.path,
                transformType: transformType,
                timestamp: new Date().toISOString()
            };

            this.pendingChanges.set(this.currentMapping.fhirPath, mappingChange);
            
            this.handler.showNotification(
                `✅ Mapping updated: ${hl7Field.path} → ${fhirField.path}`, 
                'success'
            );
            
            this.closeEnhancedModal();
            await this.applyMappingChanges();
        }

        /**
         * Apply all pending mapping changes
         */
        async applyMappingChanges() {
            console.log('🔄 Applying mapping changes:', this.pendingChanges);
            
            if (this.handler.loadInterface) {
                this.handler.loadInterface();
            }
        }

        /**
         * Clear validation results display
         */
        clearValidationResults() {
            const resultsContainer = document.getElementById('validationResults');
            if (resultsContainer) {
                resultsContainer.innerHTML = '<div class="validation-neutral">Select fields to validate mapping</div>';
            }
        }

        /**
         * Show the modal
         */
        showModal() {
            console.log('👁️ Attempting to show modal...');
            
            const modal = document.getElementById('enhancedMappingOverlay');
            if (modal) {
                console.log('✅ Modal element found, showing...');
                
                // Force show with inline styles
                modal.style.display = 'flex';
                modal.style.opacity = '1';
                modal.style.visibility = 'visible';
                
                // Add show class if it exists
                modal.classList.add('show');
                
                // Log final state
                console.log('📊 Modal final state:', {
                    display: modal.style.display,
                    opacity: modal.style.opacity,
                    visibility: modal.style.visibility,
                    zIndex: modal.style.zIndex,
                    classes: modal.className
                });
                
                // Test if modal is actually visible
                const rect = modal.getBoundingClientRect();
                console.log('📐 Modal dimensions:', {
                    width: rect.width,
                    height: rect.height,
                    top: rect.top,
                    left: rect.left,
                    bottom: rect.bottom,
                    right: rect.right
                });
                
                // Check if modal is in viewport
                const isVisible = rect.width > 0 && rect.height > 0;
                console.log('👀 Modal is visible:', isVisible);
                
                if (!isVisible) {
                    console.error('❌ Modal exists but is not visible - checking parent elements...');
                    let parent = modal.parentElement;
                    while (parent) {
                        const parentRect = parent.getBoundingClientRect();
                        console.log(`📊 Parent ${parent.tagName}:`, {
                            display: getComputedStyle(parent).display,
                            overflow: getComputedStyle(parent).overflow,
                            zIndex: getComputedStyle(parent).zIndex,
                            width: parentRect.width,
                            height: parentRect.height
                        });
                        parent = parent.parentElement;
                        if (parent === document.body) break;
                    }
                }
                
            } else {
                console.error('❌ Modal element not found - cannot show modal');
                console.log('🔍 Available elements with "mapping" in ID:', 
                    Array.from(document.querySelectorAll('[id*="mapping"]')).map(el => el.id)
                );
            }
        }

        /**
         * Close the enhanced modal
         */
        closeEnhancedModal() {
            const modal = document.getElementById('enhancedMappingOverlay');
            if (modal) {
                modal.classList.remove('show');
                setTimeout(() => modal.remove(), 300);
            }
            this.currentMapping = null;
        }

        /**
         * Load available fields for mapping
         */
        loadAvailableFields() {
            console.log('📚 Loading available fields for mapping...');
        }
    }

    // Make globally available
    window.EnhancedMappingInterface = EnhancedMappingInterface;
    
    // Add a global test function
    window.testEnhancedMapping = function() {
        if (window.enhancedMappingInterface) {
            return window.enhancedMappingInterface.testInterface();
        } else {
            console.error('❌ No enhanced mapping interface available');
            return 'No interface available';
        }
    };
    
    console.log('✅ EnhancedMappingInterface loaded and available globally');
    console.log('🧪 Test with: window.testEnhancedMapping()');

})();