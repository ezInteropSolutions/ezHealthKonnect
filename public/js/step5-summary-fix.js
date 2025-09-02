// public/js/step5-summary-fix.js
// Fix for missing summary elements in Step 5

(function() {
    'use strict';
    
    console.log('Loading Step 5 Summary Elements Fix...');
    
    // Wait for step handlers to load
    const initSummaryFix = setInterval(() => {
        if (!window.wizard) {
            return;
        }
        
        clearInterval(initSummaryFix);
        console.log('Wizard found, applying Step 5 summary fix...');
        
        // Override the SummaryStepHandler initialize method
        if (window.SummaryStepHandler) {
            const originalInitialize = window.SummaryStepHandler.prototype.initialize;
            
            window.SummaryStepHandler.prototype.initialize = function() {
                console.log('Enhanced Step 5 initialize called...');
                
                // Ensure summary elements exist
                this.ensureSummaryElements();
                
                // Call original initialize if it exists
                if (originalInitialize) {
                    try {
                        originalInitialize.call(this);
                    } catch (error) {
                        console.warn('Original initialize failed, continuing with fix:', error);
                    }
                }
                
                // Populate summary with current wizard data
                this.updateSummaryData();
            };
            
            // Add the ensureSummaryElements method
            window.SummaryStepHandler.prototype.ensureSummaryElements = function() {
                console.log('Ensuring summary elements exist...');
                
                const step5 = document.getElementById('step5');
                if (!step5) {
                    console.error('Step 5 element not found');
                    return;
                }
                
                // Check if summary elements already exist
                if (document.getElementById('summaryInterfaceName')) {
                    console.log('Summary elements already exist, skipping creation');
                    return;
                }
                
                // Create summary container
                const summaryContainer = document.createElement('div');
                summaryContainer.className = 'summary-container';
                summaryContainer.innerHTML = `
                    <div class="summary-header">
                        <h3>Interface Configuration Summary</h3>
                        <p class="summary-subtitle">Review your interface configuration before completing the wizard</p>
                    </div>
                    
                    <div class="summary-content">
                        <div class="summary-section">
                            <h4>Basic Configuration</h4>
                            <div class="summary-grid">
                                <div class="summary-row">
                                    <label>Interface Name:</label>
                                    <span id="summaryInterfaceName">-</span>
                                </div>
                                
                                <div class="summary-row">
                                    <label>Description:</label>
                                    <span id="summaryInterfaceDescription">-</span>
                                </div>
                                
                                <div class="summary-row">
                                    <label>Message Type:</label>
                                    <span id="summaryMessageType">-</span>
                                </div>
                            </div>
                        </div>
                        
                        <div class="summary-section">
                            <h4>Source & Target Configuration</h4>
                            <div class="summary-grid">
                                <div class="summary-row">
                                    <label>Source:</label>
                                    <span id="summarySource">-</span>
                                </div>
                                
                                <div class="summary-row">
                                    <label>Target:</label>
                                    <span id="summaryTarget">-</span>
                                </div>
                                
                                <div class="summary-row">
                                    <label>Segments:</label>
                                    <span id="summarySegments">-</span>
                                </div>
                                
                                <div class="summary-row">
                                    <label>Transform:</label>
                                    <span id="summaryTransform">-</span>
                                </div>
                            </div>
                        </div>
                        
                        <div class="summary-section">
                            <h4>Configuration Status</h4>
                            <div class="summary-status" id="summaryStatus">
                                <div class="status-item">
                                    <span class="status-label">Configuration:</span>
                                    <span class="status-value" id="configStatus">Pending</span>
                                </div>
                                <div class="status-item">
                                    <span class="status-label">Validation:</span>
                                    <span class="status-value" id="validationStatus">Pending</span>
                                </div>
                            </div>
                        </div>
                    </div>
                `;
                
                // Insert at the beginning of step5
                if (step5.firstChild) {
                    step5.insertBefore(summaryContainer, step5.firstChild);
                } else {
                    step5.appendChild(summaryContainer);
                }
                
                // Add CSS if not already added
                if (!document.getElementById('summary-fix-styles')) {
                    const style = document.createElement('style');
                    style.id = 'summary-fix-styles';
                    style.textContent = `
                        .summary-container {
                            padding: 24px;
                            background: linear-gradient(135deg, #f8f9fa 0%, #ffffff 100%);
                            border: 2px solid #f8bbd9;
                            border-radius: 16px;
                            margin: 20px 0;
                            box-shadow: 0 4px 6px rgba(248, 187, 217, 0.1);
                        }
                        
                        .summary-header {
                            text-align: center;
                            margin-bottom: 24px;
                            padding-bottom: 16px;
                            border-bottom: 2px solid #f1f5f9;
                        }
                        
                        .summary-header h3 {
                            margin: 0 0 8px 0;
                            color: #1e3a8a;
                            font-size: 20px;
                            font-weight: 600;
                        }
                        
                        .summary-subtitle {
                            margin: 0;
                            color: #6b7280;
                            font-size: 14px;
                        }
                        
                        .summary-section {
                            margin-bottom: 24px;
                        }
                        
                        .summary-section:last-child {
                            margin-bottom: 0;
                        }
                        
                        .summary-section h4 {
                            margin: 0 0 12px 0;
                            color: #1e3a8a;
                            font-size: 16px;
                            font-weight: 600;
                            display: flex;
                            align-items: center;
                            gap: 8px;
                        }
                        
                        .summary-section h4::before {
                            content: '•';
                            color: #f8bbd9;
                            font-size: 18px;
                        }
                        
                        .summary-grid {
                            display: grid;
                            grid-template-columns: 1fr;
                            gap: 12px;
                        }
                        
                        .summary-row {
                            display: flex;
                            justify-content: space-between;
                            align-items: center;
                            padding: 12px 16px;
                            background: white;
                            border: 1px solid #e2e8f0;
                            border-radius: 8px;
                            transition: all 0.2s ease;
                        }
                        
                        .summary-row:hover {
                            border-color: #f8bbd9;
                            transform: translateY(-1px);
                            box-shadow: 0 2px 4px rgba(248, 187, 217, 0.1);
                        }
                        
                        .summary-row label {
                            font-weight: 500;
                            color: #374151;
                            font-size: 14px;
                        }
                        
                        .summary-row span {
                            font-weight: 600;
                            color: #1e3a8a;
                            font-size: 14px;
                            text-align: right;
                            max-width: 250px;
                            overflow: hidden;
                            text-overflow: ellipsis;
                            white-space: nowrap;
                        }
                        
                        .summary-status {
                            display: grid;
                            grid-template-columns: 1fr 1fr;
                            gap: 16px;
                        }
                        
                        .status-item {
                            display: flex;
                            justify-content: space-between;
                            align-items: center;
                            padding: 16px;
                            background: white;
                            border: 2px solid #e2e8f0;
                            border-radius: 12px;
                        }
                        
                        .status-label {
                            font-weight: 500;
                            color: #374151;
                            font-size: 14px;
                        }
                        
                        .status-value {
                            font-weight: 600;
                            font-size: 14px;
                            padding: 4px 12px;
                            border-radius: 20px;
                            text-transform: uppercase;
                            font-size: 12px;
                            letter-spacing: 0.5px;
                        }
                        
                        .status-value.pending {
                            background: #fef3c7;
                            color: #92400e;
                        }
                        
                        .status-value.complete {
                            background: #dcfce7;
                            color: #166534;
                        }
                        
                        .status-value.error {
                            background: #fef2f2;
                            color: #991b1b;
                        }
                        
                        @media (max-width: 768px) {
                            .summary-container {
                                padding: 16px;
                                margin: 16px 0;
                            }
                            
                            .summary-row {
                                flex-direction: column;
                                align-items: flex-start;
                                gap: 4px;
                            }
                            
                            .summary-row span {
                                max-width: none;
                                text-align: left;
                            }
                            
                            .summary-status {
                                grid-template-columns: 1fr;
                            }
                        }
                    `;
                    document.head.appendChild(style);
                }
                
                console.log('Summary elements created successfully');
            };
            
            // Add the updateSummaryData method
            window.SummaryStepHandler.prototype.updateSummaryData = function() {
                console.log('Updating summary data...');
                
                if (!this.wizard || !this.wizard.wizardData) {
                    console.warn('No wizard data available for summary');
                    return;
                }
                
                const wizardData = this.wizard.wizardData;
                
                // Update basic configuration
                this.updateElementText('summaryInterfaceName', wizardData.name || 'Not specified');
                this.updateElementText('summaryInterfaceDescription', wizardData.description || 'No description');
                this.updateElementText('summaryMessageType', wizardData.messageType || 'Not specified');
                
                // Update source and target
                const sourceText = this.formatSourceTarget(wizardData.sourceType, wizardData.sourceConnectivity);
                const targetText = this.formatSourceTarget(wizardData.targetType, wizardData.targetConnectivity);
                
                this.updateElementText('summarySource', sourceText);
                this.updateElementText('summaryTarget', targetText);
                
                // Update segments and transform info
                const segmentCount = this.getSegmentCount();
                const transformInfo = this.getTransformInfo();
                
                this.updateElementText('summarySegments', segmentCount);
                this.updateElementText('summaryTransform', transformInfo);
                
                // Update status
                this.updateConfigurationStatus();
                
                console.log('Summary data updated successfully');
            };
            
            // Helper methods
            window.SummaryStepHandler.prototype.updateElementText = function(elementId, text) {
                const element = document.getElementById(elementId);
                if (element) {
                    element.textContent = text;
                    element.title = text; // Add tooltip for long text
                }
            };
            
            window.SummaryStepHandler.prototype.formatSourceTarget = function(type, connectivity) {
                if (!type) return 'Not configured';
                
                const formatted = type.toUpperCase();
                if (connectivity) {
                    return `${formatted} via ${connectivity.toUpperCase()}`;
                }
                return formatted;
            };
            
            window.SummaryStepHandler.prototype.getSegmentCount = function() {
                if (this.wizard.parsedHL7Data?.data?.enhancedSegments) {
                    const count = Object.keys(this.wizard.parsedHL7Data.data.enhancedSegments).length;
                    return `${count} segments processed`;
                }
                return 'No segments processed';
            };
            
            window.SummaryStepHandler.prototype.getTransformInfo = function() {
                const wizardData = this.wizard.wizardData;
                if (wizardData?.mappingRuleIds && wizardData.mappingRuleIds.length > 0) {
                    return `${wizardData.mappingRuleIds.length} mapping rules configured`;
                }
                return 'No transformation rules configured';
            };
            
            window.SummaryStepHandler.prototype.updateConfigurationStatus = function() {
                const wizardData = this.wizard.wizardData;
                
                // Check configuration completeness
                const hasBasicConfig = wizardData?.name && wizardData?.sourceType && wizardData?.targetType;
                const configStatusElement = document.getElementById('configStatus');
                
                if (configStatusElement) {
                    if (hasBasicConfig) {
                        configStatusElement.textContent = 'Complete';
                        configStatusElement.className = 'status-value complete';
                    } else {
                        configStatusElement.textContent = 'Incomplete';
                        configStatusElement.className = 'status-value error';
                    }
                }
                
                // Check validation status
                const hasValidation = wizardData?.interfaceId || (wizardData?.mappingRuleIds && wizardData.mappingRuleIds.length > 0);
                const validationStatusElement = document.getElementById('validationStatus');
                
                if (validationStatusElement) {
                    if (hasValidation) {
                        validationStatusElement.textContent = 'Validated';
                        validationStatusElement.className = 'status-value complete';
                    } else {
                        validationStatusElement.textContent = 'Pending';
                        validationStatusElement.className = 'status-value pending';
                    }
                }
            };
            
            console.log('Step 5 Summary Fix applied successfully');
            
        } else {
            console.warn('SummaryStepHandler not found, will try again...');
            // Try again in a moment
            setTimeout(() => {
                if (window.SummaryStepHandler) {
                    console.log('SummaryStepHandler found on retry, applying fix...');
                    // Re-run the enhancement
                    const event = new CustomEvent('step5FixReady');
                    document.dispatchEvent(event);
                }
            }, 1000);
        }
    }, 500);
    
    // Cleanup interval after reasonable time
    setTimeout(() => {
        clearInterval(initSummaryFix);
    }, 30000);
    
    console.log('Step 5 Summary Fix loaded');
    
})();