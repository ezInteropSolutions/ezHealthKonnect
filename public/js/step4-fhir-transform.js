// Step 4 FHIR Transformation Handler
// Enhanced FHIR transformation interface for the wizard step 4

class Step4FHIRTransform {
    constructor() {
        this.fhirResult = null;
        this.parsedHL7Data = null;
        this.initialized = false;
        this.apiBaseUrl = this.getApiBaseUrl();

        console.log('🔄 Step4FHIRTransform initialized');
    }

    // Initialize step 4 when it becomes active
    initializeStep4() {
        if (this.initialized) return;

        console.log('🎯 Initializing Step 4 FHIR transformation...');

        // Get parsed HL7 data from previous steps
        this.loadParsedHL7Data();

        // Update UI with message type
        this.updateMessageTypeDisplay();

        // Set up event listeners
        this.setupEventListeners();

        this.initialized = true;
        console.log('✅ Step 4 initialization complete');
    }

    // Load parsed HL7 data from wizard controller or optimized wizard
    loadParsedHL7Data() {
        try {
            // Try to get data from wizard controller first
            if (window.wizardController?.model?.getCurrentStepData) {
                const stepData = window.wizardController.model.getCurrentStepData();
                this.parsedHL7Data = stepData?.parsedHL7Data;
            }

            // Fallback: Try to get from optimized wizard view
            if (!this.parsedHL7Data && window.wizardView?.parsedHL7Data) {
                this.parsedHL7Data = window.wizardView.parsedHL7Data;
            }

            // Fallback: Try to get from global wizard data
            if (!this.parsedHL7Data && window.wizard?.data?.parsedHL7Data) {
                this.parsedHL7Data = window.wizard.data.parsedHL7Data;
            }

            console.log('📊 Loaded HL7 data for FHIR transformation:', !!this.parsedHL7Data);
            if (this.parsedHL7Data) {
                console.log('   - Message type:', this.parsedHL7Data.detectedMessageType);
                console.log('   - Segments:', Object.keys(this.parsedHL7Data.enhancedSegments || {}));
            }

        } catch (error) {
            console.error('❌ Error loading HL7 data:', error);
        }
    }

    // Update message type display
    updateMessageTypeDisplay() {
        const messageTypeElement = document.getElementById('fhir-message-type');
        if (messageTypeElement && this.parsedHL7Data?.detectedMessageType) {
            messageTypeElement.textContent = this.parsedHL7Data.detectedMessageType;
        }
    }

    // Set up event listeners for step 4
    setupEventListeners() {
        // Strategy tab switching
        const strategyTabs = document.querySelectorAll('.strategy-tab');
        strategyTabs.forEach(tab => {
            tab.addEventListener('click', (e) => {
                this.switchStrategy(e.target.dataset.strategy);
            });
        });
    }

    // Switch mapping strategy
    switchStrategy(strategy) {
        // Update active tab
        document.querySelectorAll('.strategy-tab').forEach(tab => {
            tab.classList.remove('active');
        });
        document.querySelector(`[data-strategy="${strategy}"]`).classList.add('active');

        console.log('📋 Switched to strategy:', strategy);
    }

    // Start FHIR transformation
    async startFHIRTransformation() {
        console.log('🔄 Starting FHIR transformation...');

        if (!this.parsedHL7Data) {
            this.showError('No HL7 data available for transformation. Please complete step 2 first.');
            return;
        }

        // Update UI state
        this.showLoadingState();
        this.updateConfigStatus('transforming');

        try {
            const requestData = {
                parsedHL7Data: this.parsedHL7Data,
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
                this.fhirResult = await response.json();
                console.log('✅ FHIR transformation successful:', this.fhirResult);
                this.showTransformationResults(this.fhirResult);
                this.updateConfigStatus('completed');
            } else {
                const errorText = await response.text();
                console.error('❌ API Error:', response.status, errorText);
                this.showError(`Transformation failed: ${response.status} - ${errorText}`);
                this.updateConfigStatus('error');
            }
        } catch (error) {
            console.error('❌ Failed to transform to FHIR:', error);
            this.showError(`Network error: ${error.message}`);
            this.updateConfigStatus('error');
        }
    }

    // Show loading state
    showLoadingState() {
        const resultsContainer = document.getElementById('fhir-transformation-results');
        if (!resultsContainer) return;

        resultsContainer.innerHTML = `
            <div class="transformation-loading">
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

    // Show transformation results
    showTransformationResults(fhirResult) {
        const resultsContainer = document.getElementById('fhir-transformation-results');
        if (!resultsContainer) return;

        const resources = fhirResult.fhirResources || [];
        const atomicMappings = fhirResult.atomicMappings || [];

        resultsContainer.innerHTML = `
            <div class="transformation-success">
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
                            <button onclick="window.step4FHIR.viewFHIRJSON()"
                                    style="padding: 8px 16px; background: #16a34a; color: white; border: none; border-radius: 6px; font-size: 12px; cursor: pointer; font-weight: 500;">
                                📄 View JSON
                            </button>
                        </div>
                    </div>
                </div>

                <!-- FHIR Resources Grid -->
                <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); gap: 16px; margin-bottom: 24px;">
                    ${resources.map((resource, index) => this.renderFHIRResourceCard(resource, index)).join('')}
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

        // Show the JSON view button
        const jsonButton = document.getElementById('btn-view-fhir-json');
        if (jsonButton) {
            jsonButton.style.display = 'inline-flex';
        }

        // Show enhanced mapping interface
        const enhancedInterface = document.getElementById('enhanced-mapping-interface');
        if (enhancedInterface) {
            enhancedInterface.style.display = 'block';
        }
    }

    // Render FHIR resource card
    renderFHIRResourceCard(resource, index) {
        const resourceType = resource.resourceType;
        const icon = this.getFHIRResourceIcon(resourceType);
        const summary = this.getFHIRResourceSummary(resource);

        return `
            <div class="fhir-resource-card" style="background: white; border: 2px solid #e2e8f0; border-radius: 12px; overflow: hidden; transition: all 0.3s ease;">
                <div style="background: linear-gradient(135deg, #1e3a8a 0%, #1e40af 100%); color: white; padding: 16px;">
                    <div style="display: flex; justify-content: space-between; align-items: center;">
                        <div style="display: flex; align-items: center; gap: 12px;">
                            <span style="font-size: 24px;">${icon}</span>
                            <div>
                                <h5 style="margin: 0; font-size: 16px; font-weight: 600;">${resourceType}</h5>
                                <p style="margin: 4px 0 0 0; font-size: 12px; opacity: 0.9;">Resource ${index + 1}</p>
                            </div>
                        </div>
                        <button onclick="window.step4FHIR.viewResourceDetails(${index})"
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

    // Get FHIR resource icon
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

    // Get FHIR resource summary
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

    // View FHIR JSON in modal
    viewFHIRJSON() {
        if (!this.fhirResult) {
            console.warn('⚠️ No FHIR result available');
            return;
        }
        this.showJSONModal(this.fhirResult, 'Complete FHIR Transformation Result');
    }

    // View specific resource details
    viewResourceDetails(index) {
        if (!this.fhirResult?.fhirResources?.[index]) {
            console.warn('⚠️ Resource not found');
            return;
        }

        const resource = this.fhirResult.fhirResources[index];
        this.showJSONModal(resource, `${resource.resourceType} Resource`);
    }

    // Show JSON in modal
    showJSONModal(data, title = 'FHIR JSON') {
        // Create modal if it doesn't exist
        let modal = document.getElementById('fhirJsonModal');
        if (!modal) {
            modal = document.createElement('div');
            modal.id = 'fhirJsonModal';
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
                    <button onclick="window.step4FHIR.closeFHIRModal()" style="background: none; border: none; font-size: 24px; cursor: pointer; color: #6b7280;">×</button>
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
    closeFHIRModal() {
        const modal = document.getElementById('fhirJsonModal');
        if (modal) {
            modal.style.display = 'none';
        }
    }

    // Show error state
    showError(message) {
        const resultsContainer = document.getElementById('fhir-transformation-results');
        if (!resultsContainer) return;

        resultsContainer.innerHTML = `
            <div class="transformation-error">
                <div style="text-align: center; padding: 60px 20px;">
                    <div style="font-size: 64px; margin-bottom: 24px; color: #dc2626;">⚠️</div>
                    <div style="font-size: 18px; font-weight: 600; margin-bottom: 12px; color: #dc2626;">Transformation Failed</div>
                    <div style="font-size: 14px; color: #6b7280; margin-bottom: 32px; max-width: 400px; margin-left: auto; margin-right: auto; line-height: 1.5;">
                        ${message}
                    </div>
                    <button onclick="window.step4FHIR.startFHIRTransformation()"
                            style="padding: 12px 24px; background: #dc2626; color: white; border: none; border-radius: 8px; font-size: 14px; font-weight: 600; cursor: pointer;">
                        🔄 Retry Transformation
                    </button>
                </div>
            </div>
        `;
    }

    // Update configuration status
    updateConfigStatus(status) {
        const statusElement = document.getElementById('fhir-config-status');
        if (!statusElement) return;

        const statusConfig = {
            'ready': { text: 'Ready', class: 'unsaved' },
            'transforming': { text: 'Transforming', class: 'modified' },
            'completed': { text: 'Completed', class: 'saved' },
            'error': { text: 'Error', class: 'unsaved' }
        };

        const config = statusConfig[status] || statusConfig.ready;
        statusElement.textContent = config.text;
        statusElement.className = `config-status ${config.class}`;
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

// Initialize global instance
window.step4FHIR = new Step4FHIRTransform();

// Global functions for HTML onclick handlers
window.startFHIRTransformation = function() {
    window.step4FHIR.startFHIRTransformation();
};

window.viewFHIRJSON = function() {
    window.step4FHIR.viewFHIRJSON();
};

window.editFieldMappings = function() {
    console.log('🔧 Edit field mappings requested');
    // This could open an advanced mapping interface
    AppDialogs.toast('Advanced field mapping interface coming soon.', 'info');
};

window.validateTransformation = function() {
    console.log('✅ Validate transformation requested');
    // This could run validation checks
    AppDialogs.toast('FHIR validation checks coming soon.', 'info');
};

// Hook into wizard step changes
document.addEventListener('DOMContentLoaded', function() {
    // Listen for wizard step changes
    const originalShowStep = window.showStep;
    if (originalShowStep) {
        window.showStep = function(stepNumber) {
            const result = originalShowStep.call(this, stepNumber);

            // Initialize step 4 when it becomes active
            if (stepNumber === 4) {
                setTimeout(() => {
                    window.step4FHIR.initializeStep4();
                }, 100);
            }

            return result;
        };
    }
});

console.log('📝 Step 4 FHIR Transform module loaded');