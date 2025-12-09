// WizardFHIRTransform.js - FHIR Transformation for Optimized Wizard
// Handles step 4 FHIR transformation in the optimized wizard system

class WizardFHIRTransform {
    constructor() {
        this.fhirResult = null;
        this.isTransforming = false;
        this.apiBaseUrl = this.getApiBaseUrl();

        console.log('🔄 WizardFHIRTransform initialized for optimized wizard');
    }

    // Start FHIR transformation using data from optimized wizard
    async startOptimizedFHIRTransformation() {
        console.log('🔄 Starting optimized FHIR transformation...');

        if (this.isTransforming) {
            console.warn('⚠️ Transformation already in progress');
            return;
        }

        // Get parsed HL7 data from wizard controller
        const parsedHL7Data = this.getParsedHL7DataFromWizard();
        if (!parsedHL7Data) {
            this.showOptimizedError('No HL7 data available for transformation. Please complete step 2 first.');
            return;
        }

        this.isTransforming = true;
        this.updateOptimizedConfigStatus('transforming');
        this.showOptimizedLoadingState();

        try {
            // parsedHL7Data already contains the .data from the parse result
            const requestData = {
                parsedHL7Data: parsedHL7Data,  // Direct use of parsed data (already extracted from result.data)
                createBundle: true,
                validationMode: 'strict',
                fhirVersion: 'R4',
                targetProfile: 'base'
            };

            console.log('📡 Sending transform request to:', `${this.apiBaseUrl}/api/fhir/test-transform-v3`);
            console.log('📊 Request payload:', JSON.stringify(requestData, null, 2));

            const response = await fetch(`${this.apiBaseUrl}/api/fhir/test-transform-v3`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify(requestData)
            });

            if (response.ok) {
                this.fhirResult = await response.json();
                console.log('📋 FHIR transformation response:', this.fhirResult);

                // Check if the transformation generated mappings and resources (regardless of validation status)
                if (this.fhirResult.atomicMappings && this.fhirResult.atomicMappings.length > 0) {
                    console.log('✅ FHIR transformation completed with atomic mappings:', this.fhirResult.atomicMappings.length);
                    console.log('📊 FHIR resources generated:', this.fhirResult.fhirResources?.length || 0);

                    // Store the result in the wizard model for Step 4 access
                    this.storeFHIRResultInWizard(this.fhirResult);

                    // Update the wizard view with results
                    this.showOptimizedTransformationResults(this.fhirResult);

                    // Determine status based on validation errors but always show mappings
                    if (this.fhirResult.success) {
                        this.updateOptimizedConfigStatus('completed');
                        console.log('✅ Transformation completed successfully');
                    } else {
                        this.updateOptimizedConfigStatus('completed-with-warnings');
                        console.log('⚠️ Transformation completed with validation warnings');
                    }

                    // Show enhanced mapping interface
                    this.showEnhancedMappingInterface();

                    // Show JSON view button
                    const jsonButton = document.getElementById('btn-view-fhir-json');
                    if (jsonButton) {
                        jsonButton.style.display = 'inline-flex';
                    }
                } else {
                    // Backend returned no mappings - this is a real failure
                    const errorMsg = this.fhirResult.errors && this.fhirResult.errors.length > 0
                        ? this.fhirResult.errors.join(', ')
                        : 'Transformation completed but no atomic mappings were generated';
                    console.error('❌ FHIR transformation failed:', errorMsg);
                    this.showOptimizedError(`Transformation failed: ${errorMsg}`);
                    this.updateOptimizedConfigStatus('error');
                }

            } else {
                const errorText = await response.text();
                console.error('❌ API Error:', response.status, errorText);
                this.showOptimizedError(`Transformation failed: ${response.status} - ${errorText}`);
                this.updateOptimizedConfigStatus('error');
            }
        } catch (error) {
            console.error('❌ Failed to transform to FHIR:', error);
            this.showOptimizedError(`Network error: ${error.message}`);
            this.updateOptimizedConfigStatus('error');
        } finally {
            this.isTransforming = false;
        }
    }

    // Store FHIR transformation result in the wizard model for Step 4 access
    storeFHIRResultInWizard(fhirResult) {
        try {
            if (window.wizardController?.model?.data) {
                console.log('💾 Storing FHIR result in wizard model for Step 4 access');
                window.wizardController.model.data.fhirTransformResult = fhirResult;
                console.log('✅ FHIR result stored successfully');
                console.log('📊 Stored mappings count:', fhirResult.atomicMappings?.length || 0);
                console.log('📊 Current step:', window.wizardController.model.currentStep);

                // Refresh the current step view to show the new mapping count
                const currentStep = window.wizardController.model.currentStep;

                // Step 3 (index 2) or Step 4 (index 3) - both need to show mapping data
                if (currentStep === 2 || currentStep === 3) {
                    console.log(`🔄 Refreshing Step ${currentStep + 1} view with new FHIR transformation result`);
                    const currentData = window.wizardController.model.getCurrentStepData();
                    console.log('📦 Current step data before render:', {
                        hasFhirTransformResult: !!currentData.fhirTransformResult,
                        mappingCount: currentData.fhirTransformResult?.atomicMappings?.length || 0
                    });

                    // Wait for any ongoing animation to complete before updating UI
                    const attemptUpdate = () => {
                        if (window.wizardController.view.isAnimating) {
                            console.log('⏳ Animation in progress, waiting 500ms before updating...');
                            setTimeout(attemptUpdate, 500);
                        } else {
                            console.log('✨ Animation complete, updating mapping display now');

                            // Update mapping count in the header
                            const mappingCountElement = document.getElementById('mapping-count');
                            if (mappingCountElement) {
                                mappingCountElement.textContent = fhirResult.atomicMappings?.length || 0;
                                console.log('✅ Updated mapping count badge');
                            }

                            // Update the mapping container content
                            const mappingContainer = document.getElementById('fhir-mapping-container');
                            console.log('🔍 Looking for mapping container:', {
                                found: !!mappingContainer,
                                currentHTML: mappingContainer?.innerHTML?.substring(0, 100)
                            });

                            if (mappingContainer) {
                                console.log('🔄 Updating mapping container with new mappings');
                                console.log('📊 Generating mapping content for:', {
                                    mappingCount: currentData.fhirTransformResult?.atomicMappings?.length,
                                    hasTransformResult: !!currentData.fhirTransformResult
                                });

                                const newContent = window.wizardController.view.getFHIRMappingContent(currentData);
                                console.log('📄 Generated content length:', newContent.length, 'chars');
                                console.log('📄 Content preview:', newContent.substring(0, 200));

                                mappingContainer.innerHTML = newContent;
                                console.log('✅ Mapping container innerHTML updated successfully');
                                console.log('✅ New container preview:', mappingContainer.innerHTML.substring(0, 200));

                                // Force browser repaint to ensure visual update
                                void mappingContainer.offsetHeight;
                                mappingContainer.style.opacity = '0.99';
                                setTimeout(() => {
                                    mappingContainer.style.opacity = '1';
                                    console.log('🎨 Forced visual repaint completed');
                                }, 10);
                            } else {
                                console.warn('⚠️ Mapping container not found, doing full re-render');
                                console.log('🔄 Available elements:', {
                                    wizardContent: !!document.getElementById('wizard-content'),
                                    stepContent: !!document.querySelector('.wizard-step-content'),
                                    allIds: Array.from(document.querySelectorAll('[id]')).map(el => el.id)
                                });
                                window.wizardController.view.renderStep(currentStep, currentData);
                            }

                            // Re-validate the step to enable Next button
                            if (window.wizardController.view.validateCurrentStep) {
                                console.log('🔍 Re-validating step after FHIR transformation...');
                                window.wizardController.view.validateCurrentStep();
                            }
                        }
                    };

                    attemptUpdate();
                }
            } else {
                console.warn('⚠️ Could not store FHIR result - wizard model not available');
            }
        } catch (error) {
            console.error('❌ Error storing FHIR result:', error);
        }
    }

    // Get parsed HL7 data from the optimized wizard system
    getParsedHL7DataFromWizard() {
        try {
            // Try different sources for parsed HL7 data
            if (window.wizardController?.model?.data?.parsedHL7Data) {
                return window.wizardController.model.data.parsedHL7Data;
            }

            if (window.wizardView?.parsedHL7Data) {
                return window.wizardView.parsedHL7Data;
            }

            if (window.wizard?.data?.parsedHL7Data) {
                return window.wizard.data.parsedHL7Data;
            }

            console.warn('⚠️ No parsed HL7 data found in wizard system');
            return null;
        } catch (error) {
            console.error('❌ Error getting parsed HL7 data:', error);
            return null;
        }
    }

    // Show loading state in optimized wizard
    showOptimizedLoadingState() {
        const resultsContainer = document.getElementById('optimized-fhir-transformation-results');
        if (!resultsContainer) return;

        resultsContainer.innerHTML = `
            <div style="text-align: center; padding: 60px 20px;">
                <div style="position: relative; display: inline-block; margin-bottom: 24px;">
                    <div style="width: 60px; height: 60px; border: 4px solid #e2e8f0; border-top: 4px solid #1e3a8a; border-radius: 50%; animation: spin 1s linear infinite;"></div>
                </div>
                <div style="font-size: 18px; font-weight: 600; margin-bottom: 12px; color: #1e3a8a;">Transforming to FHIR...</div>
                <div style="font-size: 14px; color: #6b7280; margin-bottom: 24px;">Converting HL7 segments to FHIR R4 resources</div>
                <div style="background: #f8fafc; border-radius: 8px; padding: 16px; max-width: 400px; margin: 0 auto;">
                    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px;">
                        <span style="font-size: 12px; color: #64748b;">Processing...</span>
                        <span style="font-size: 12px; color: #1e3a8a; font-weight: 600;">75%</span>
                    </div>
                    <div style="width: 100%; height: 6px; background: #e2e8f0; border-radius: 3px; overflow: hidden;">
                        <div style="width: 75%; height: 100%; background: linear-gradient(90deg, #1e3a8a, #1e40af); animation: progress-slide 2s ease-in-out infinite;"></div>
                    </div>
                </div>
                <style>
                    @keyframes spin {
                        0% { transform: rotate(0deg); }
                        100% { transform: rotate(360deg); }
                    }
                    @keyframes progress-slide {
                        0%, 100% { transform: translateX(-10px); }
                        50% { transform: translateX(10px); }
                    }
                </style>
            </div>
        `;
    }

    // Show transformation results in optimized wizard
    showOptimizedTransformationResults(fhirResult) {
        const resultsContainer = document.getElementById('optimized-fhir-transformation-results');
        if (!resultsContainer) return;

        // Use the WizardView method to render the results
        if (window.wizardView && typeof window.wizardView.getFHIRTransformationResults === 'function') {
            resultsContainer.innerHTML = window.wizardView.getFHIRTransformationResults(fhirResult);
        } else {
            console.warn('⚠️ WizardView FHIR methods not available, using fallback');
            resultsContainer.innerHTML = this.getFallbackResultsHTML(fhirResult);
        }
    }

    // Fallback results HTML if WizardView methods aren't available
    getFallbackResultsHTML(fhirResult) {
        const resources = fhirResult.fhirResources || [];
        const atomicMappings = fhirResult.atomicMappings || [];

        return `
            <div style="padding: 20px;">
                <div style="background: linear-gradient(135deg, #f0fdf4 0%, #ffffff 100%); border: 2px solid #bbf7d0; border-radius: 12px; padding: 20px; margin-bottom: 24px;">
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

                <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); gap: 16px;">
                    ${resources.map((resource, index) => `
                        <div style="background: white; border: 2px solid #e2e8f0; border-radius: 12px; overflow: hidden;">
                            <div style="background: linear-gradient(135deg, #1e3a8a 0%, #1e40af 100%); color: white; padding: 16px;">
                                <h5 style="margin: 0; font-size: 16px; font-weight: 600;">${resource.resourceType}</h5>
                                <p style="margin: 4px 0 0 0; font-size: 12px; opacity: 0.9;">Resource ${index + 1}</p>
                            </div>
                            <div style="padding: 16px;">
                                <div style="color: #6b7280;">ID: ${resource.id || 'N/A'}</div>
                            </div>
                        </div>
                    `).join('')}
                </div>
            </div>
        `;
    }

    // Show enhanced mapping interface
    showEnhancedMappingInterface() {
        const enhancedInterface = document.getElementById('optimized-enhanced-mapping-interface');
        if (enhancedInterface) {
            enhancedInterface.style.display = 'block';
        }
    }

    // Show error state in optimized wizard
    showOptimizedError(message) {
        const resultsContainer = document.getElementById('optimized-fhir-transformation-results');
        if (!resultsContainer) return;

        resultsContainer.innerHTML = `
            <div style="text-align: center; padding: 60px 20px;">
                <div style="font-size: 64px; margin-bottom: 24px; color: #dc2626;">⚠️</div>
                <div style="font-size: 18px; font-weight: 600; margin-bottom: 12px; color: #dc2626;">Transformation Failed</div>
                <div style="font-size: 14px; color: #6b7280; margin-bottom: 32px; max-width: 400px; margin-left: auto; margin-right: auto; line-height: 1.5;">
                    ${message}
                </div>
                <button onclick="window.startOptimizedFHIRTransformation()"
                        style="padding: 12px 24px; background: #dc2626; color: white; border: none; border-radius: 8px; font-size: 14px; font-weight: 600; cursor: pointer;">
                    🔄 Retry Transformation
                </button>
            </div>
        `;
    }

    // Update configuration status
    updateOptimizedConfigStatus(status) {
        const statusElement = document.getElementById('fhir-config-status');
        if (!statusElement) return;

        const statusConfig = {
            'ready': { text: 'Ready', class: 'status-ready' },
            'transforming': { text: 'Transforming', class: 'status-transforming' },
            'completed': { text: 'Completed', class: 'status-completed' },
            'error': { text: 'Error', class: 'status-error' }
        };

        const config = statusConfig[status] || statusConfig.ready;
        statusElement.textContent = config.text;

        // Update background color based on status
        const bgColors = {
            'ready': '#fef2f2',
            'transforming': '#fef3c7',
            'completed': '#f0fdf4',
            'error': '#fef2f2'
        };

        const textColors = {
            'ready': '#dc2626',
            'transforming': '#d97706',
            'completed': '#16a34a',
            'error': '#dc2626'
        };

        statusElement.style.backgroundColor = bgColors[status];
        statusElement.style.color = textColors[status];
    }

    // View FHIR JSON in modal
    viewOptimizedFHIRJSON() {
        if (!this.fhirResult) {
            console.warn('⚠️ No FHIR result available');
            return;
        }
        this.showOptimizedJSONModal(this.fhirResult, 'Complete FHIR Transformation Result');
    }

    // View specific resource details
    viewOptimizedResourceDetails(index) {
        if (!this.fhirResult?.fhirResources?.[index]) {
            console.warn('⚠️ Resource not found');
            return;
        }

        const resource = this.fhirResult.fhirResources[index];
        this.showOptimizedJSONModal(resource, `${resource.resourceType} Resource`);
    }

    // Show JSON in modal for optimized wizard
    showOptimizedJSONModal(data, title = 'FHIR JSON') {
        // Create modal if it doesn't exist
        let modal = document.getElementById('optimizedFhirJsonModal');
        if (!modal) {
            modal = document.createElement('div');
            modal.id = 'optimizedFhirJsonModal';
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
            document.body.appendChild(modal);
        }

        modal.innerHTML = `
            <div style="background: white; border-radius: 12px; max-width: 90%; width: 900px; max-height: 80vh; overflow: hidden; display: flex; flex-direction: column;">
                <div style="padding: 20px; border-bottom: 1px solid #e2e8f0; display: flex; justify-content: space-between; align-items: center;">
                    <h3 style="margin: 0; font-size: 18px; font-weight: 600; color: #1e3a8a;">${title}</h3>
                    <button onclick="window.closeOptimizedFHIRModal()" style="background: none; border: none; font-size: 24px; cursor: pointer; color: #6b7280;">×</button>
                </div>
                <div style="flex: 1; overflow-y: auto; padding: 20px; background: #f9fafb;">
                    <pre style="font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace; font-size: 12px; line-height: 1.6; margin: 0; white-space: pre-wrap; word-wrap: break-word;">${JSON.stringify(data, null, 2)}</pre>
                </div>
            </div>
        `;

        modal.style.display = 'flex';

        // Close on outside click
        modal.onclick = function(e) {
            if (e.target === modal) {
                modal.style.display = 'none';
            }
        };
    }

    // Close FHIR modal
    closeOptimizedFHIRModal() {
        const modal = document.getElementById('optimizedFhirJsonModal');
        if (modal) {
            modal.style.display = 'none';
        }
    }

    // Get API base URL
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
        return url;
    }
}

// Initialize global instance for optimized wizard
window.optimizedFHIRTransform = new WizardFHIRTransform();

// Global functions for HTML onclick handlers in optimized wizard
window.startOptimizedFHIRTransformation = function() {
    window.optimizedFHIRTransform.startOptimizedFHIRTransformation();
};

window.viewOptimizedFHIRJSON = function() {
    window.optimizedFHIRTransform.viewOptimizedFHIRJSON();
};

window.viewOptimizedResourceDetails = function(index) {
    window.optimizedFHIRTransform.viewOptimizedResourceDetails(index);
};

window.closeOptimizedFHIRModal = function() {
    window.optimizedFHIRTransform.closeOptimizedFHIRModal();
};

window.editOptimizedFieldMappings = function() {
    console.log('🔧 Edit field mappings requested for optimized wizard');
    // This could open an advanced mapping interface
    alert('Advanced field mapping interface would open here');
};

window.validateOptimizedTransformation = function() {
    console.log('✅ Validate transformation requested for optimized wizard');
    // This could run validation checks
    alert('FHIR validation checks would run here');
};

console.log('📝 Optimized Wizard FHIR Transform module loaded');