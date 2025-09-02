// controllers/wizardController.js
// Controller layer with format + connectivity architecture
// FIXED: Separate message format from transport connectivity

const interfaceService = require('../services/interfaceService');
const auditService = require('../services/auditService');
const { v4: uuidv4 } = require('uuid');

class WizardController {
    /**
     * Constructor - FIXED: Bind methods to preserve 'this' context for Express route handlers
     */
    constructor() {
        // Bind all Express route handler methods to ensure proper 'this' context
        this.saveConfiguration = this.saveConfiguration.bind(this);
        this.activateInterface = this.activateInterface.bind(this);
        this.completeWizard = this.completeWizard.bind(this);
        this.listInterfaces = this.listInterfaces.bind(this);
        this.getInterface = this.getInterface.bind(this);
        this.deleteInterface = this.deleteInterface.bind(this);
        this.getInterfaceStats = this.getInterfaceStats.bind(this);
        this.checkDuplicateName = this.checkDuplicateName.bind(this);
    }

    /**
     * Map UI source types to message formats and connectivity
     * SINGLE SOURCE OF TRUTH for format/connectivity mapping
     */
    mapSourceTypeAndConnectivity(uiSourceType, sourceConfig = {}) {
        const mappings = {
            'hl7v2': { 
                format: 'hl7', 
                connectivity: 'tcp' 
            },
            'hl7': { 
                format: 'hl7', 
                connectivity: 'tcp' 
            },
            'file': { 
                format: 'flatfile', 
                connectivity: 'file' 
            },
            'http': { 
                format: 'hl7',  // HTTP usually receives HL7
                connectivity: 'http' 
            },
            'database': { 
                format: 'database', 
                connectivity: 'database' 
            },
            'manual': { 
                format: 'hl7', 
                connectivity: 'tcp' 
            }
        };
        
        const mapping = mappings[uiSourceType] || { format: 'hl7', connectivity: 'tcp' };
        
        console.log(`🔧 Source mapping: ${uiSourceType} → format: ${mapping.format}, connectivity: ${mapping.connectivity}`);
        
        return mapping;
    }

    /**
     * Map UI target types to message formats and connectivity
     */
    mapTargetTypeAndConnectivity(uiTargetType, targetConfig = {}) {
        const mappings = {
            'fhir': { 
                format: 'fhir', 
                connectivity: 'http' 
            },
            'database': { 
                format: 'database', 
                connectivity: 'database' 
            },
            'file': { 
                format: 'flatfile', 
                connectivity: 'file' 
            },
            'http': { 
                format: 'fhir',  // HTTP usually sends FHIR
                connectivity: 'http' 
            },
            'hl7': { 
                format: 'hl7', 
                connectivity: 'tcp' 
            }
        };
        
        const mapping = mappings[uiTargetType] || { format: 'fhir', connectivity: 'http' };
        
        console.log(`🔧 Target mapping: ${uiTargetType} → format: ${mapping.format}, connectivity: ${mapping.connectivity}`);
        
        return mapping;
    }

    /**
     * Map status values to database constraint values
     * SINGLE SOURCE OF TRUTH for status mapping
     */
    mapStatusToDatabase(status) {
        const statusMapping = {
            'active': 'running',      // UI active → DB running
            'inactive': 'stopped',    // UI inactive → DB stopped  
            'running': 'running',     // Already valid
            'stopped': 'stopped',     // Already valid
            'paused': 'paused',       // Already valid
            'error': 'error',         // Already valid
            'draft': 'draft'          // Already valid
        };
        
        const mappedStatus = statusMapping[status] || 'draft';
        
        if (status !== mappedStatus) {
            console.log(`🔧 Status mapped: ${status} → ${mappedStatus}`);
        }
        
        return mappedStatus;
    }

    /**
     * Save wizard configuration
     */
    async saveConfiguration(req, res) {
        try {
            const { wizardData } = req.body;
            const userId = req.session.user.id;
            const userEmail = req.session.user.email;

            console.log('\n=== SAVE WIZARD CONFIGURATION ===');
            console.log('📋 User:', userEmail);
            console.log('📋 Raw wizard data keys:', Object.keys(wizardData || {}));

            if (!wizardData || !wizardData.name) {
                return res.status(400).json({
                    success: false,
                    error: 'Invalid wizard data: name is required'
                });
            }

            // Map UI values to backend format
            const sourceMapping = this.mapSourceTypeAndConnectivity(wizardData.sourceType, wizardData.sourceConfig);
            const targetMapping = this.mapTargetTypeAndConnectivity(wizardData.targetType, wizardData.targetConfig);
            const mappedStatus = this.mapStatusToDatabase(wizardData.status || 'draft');

            const mappedData = {
                ...wizardData,
                sourceType: sourceMapping.format,
                sourceConnectivity: sourceMapping.connectivity,
                targetType: targetMapping.format,
                targetConnectivity: targetMapping.connectivity,
                status: mappedStatus
            };

            console.log('🔄 Mapped wizard data:', {
                source: `${wizardData.sourceType} → ${sourceMapping.format}/${sourceMapping.connectivity}`,
                target: `${wizardData.targetType} → ${targetMapping.format}/${targetMapping.connectivity}`,
                status: `${wizardData.status || 'draft'} → ${mappedStatus}`
            });

            const result = await interfaceService.saveWizardConfiguration(mappedData, userId, userEmail);

            await auditService.logActivity({
                userId: userId,
                action: 'wizard_config_saved',
                resource: 'interface',
                resourceId: result.interfaceId,
                details: `Configuration saved for interface: ${wizardData.name}`,
                metadata: {
                    interfaceName: wizardData.name,
                    sourceType: sourceMapping.format,
                    targetType: targetMapping.format
                }
            });

            res.json({
                success: true,
                data: {
                    interfaceId: result.interfaceId,
                    message: 'Configuration saved successfully',
                    status: 'draft',
                    nextStep: 'activation'
                }
            });

        } catch (error) {
            console.error('❌ Save configuration error:', error);
            
            res.status(500).json({
                success: false,
                error: error.message || 'Failed to save configuration'
            });
        }
    }

    /**
     * Activate interface (make it ready to run)
     */
    async activateInterface(req, res) {
        try {
            const { interfaceId } = req.body;
            const userId = req.session.user.id;
            const userEmail = req.session.user.email;

            console.log('\n=== ACTIVATE INTERFACE ===');
            console.log('🚀 Interface ID:', interfaceId);
            console.log('🚀 User:', userEmail);

            if (!interfaceId) {
                return res.status(400).json({
                    success: false,
                    error: 'Interface ID is required'
                });
            }

            const result = await interfaceService.activateInterface(interfaceId, userId);

            await auditService.logActivity({
                userId: userId,
                action: 'interface_activated',
                resource: 'interface',
                resourceId: interfaceId,
                details: `Interface activated: ${result.name}`,
                metadata: {
                    interfaceName: result.name,
                    previousStatus: result.previousStatus,
                    newStatus: 'active'
                }
            });

            res.json({
                success: true,
                data: {
                    interfaceId: interfaceId,
                    name: result.name,
                    status: 'active',
                    message: 'Interface activated successfully'
                }
            });

        } catch (error) {
            console.error('❌ Activate interface error:', error);
            
            res.status(500).json({
                success: false,
                error: error.message || 'Failed to activate interface'
            });
        }
    }

    /**
     * Complete wizard (save + activate in one step)
     */
    async completeWizard(req, res) {
        try {
            const { wizardData } = req.body;
            const userId = req.session.user.id;
            const userEmail = req.session.user.email;

            console.log('\n=== COMPLETE WIZARD ===');
            console.log('🎯 User:', userEmail);
            console.log('🎯 Interface name:', wizardData?.name);

            if (!wizardData || !wizardData.name) {
                return res.status(400).json({
                    success: false,
                    error: 'Invalid wizard data: name is required'
                });
            }

            // Map UI values to backend format (same as saveConfiguration)
            const sourceMapping = this.mapSourceTypeAndConnectivity(wizardData.sourceType, wizardData.sourceConfig);
            const targetMapping = this.mapTargetTypeAndConnectivity(wizardData.targetType, wizardData.targetConfig);
            
            const mappedData = {
                ...wizardData,
                sourceType: sourceMapping.format,
                sourceConnectivity: sourceMapping.connectivity,
                targetType: targetMapping.format,
                targetConnectivity: targetMapping.connectivity,
                status: 'active'  // Complete wizard creates active interface
            };

            console.log('🔄 Complete wizard mapping:', {
                source: `${wizardData.sourceType} → ${sourceMapping.format}/${sourceMapping.connectivity}`,
                target: `${wizardData.targetType} → ${targetMapping.format}/${targetMapping.connectivity}`,
                status: 'active'
            });

            const result = await interfaceService.completeWizard(mappedData, userId, userEmail);

            await auditService.logActivity({
                userId: userId,
                action: 'wizard_completed',
                resource: 'interface',
                resourceId: result.interfaceId,
                details: `Wizard completed for interface: ${wizardData.name}`,
                metadata: {
                    interfaceName: wizardData.name,
                    sourceType: sourceMapping.format,
                    targetType: targetMapping.format,
                    finalStatus: 'active'
                }
            });

            res.json({
                success: true,
                data: {
                    interfaceId: result.interfaceId,
                    name: wizardData.name,
                    status: 'active',
                    message: 'Wizard completed successfully! Interface is now active.'
                }
            });
            
        } catch (error) {
            console.error('❌ Wizard completion error:', error);
            
            res.status(500).json({
                success: false,
                error: error.message || 'Failed to complete wizard'
            });
        }
    }
    
    /**
     * List user interfaces
     */
    async listInterfaces(req, res) {
        try {
            const userId = req.session.user.id;
            
            const interfaces = await interfaceService.getUserInterfaces(userId);
            
            res.json({
                success: true,
                data: interfaces,
                count: interfaces.length
            });
            
        } catch (error) {
            console.error('❌ List interfaces error:', error);
            
            res.status(500).json({
                success: false,
                error: error.message || 'Failed to list interfaces'
            });
        }
    }
    
    /**
     * Get interface details
     */
    async getInterface(req, res) {
        try {
            const userId = req.session.user.id;
            const interfaceId = req.params.id;
            
            if (!interfaceId) {
                return res.status(400).json({
                    success: false,
                    error: 'Interface ID is required'
                });
            }
            
            const interfaceData = await interfaceService.getInterfaceById(interfaceId, userId);
            
            if (!interfaceData) {
                return res.status(404).json({
                    success: false,
                    error: 'Interface not found'
                });
            }
            
            res.json({
                success: true,
                data: interfaceData
            });
            
        } catch (error) {
            console.error('❌ Get interface error:', error);
            
            res.status(500).json({
                success: false,
                error: error.message || 'Failed to get interface'
            });
        }
    }
    
    /**
     * Delete interface
     */
    async deleteInterface(req, res) {
        try {
            const userId = req.session.user.id;
            const interfaceId = req.params.id;
            const userEmail = req.session.user.email;
            
            if (!interfaceId) {
                return res.status(400).json({
                    success: false,
                    error: 'Interface ID is required'
                });
            }
            
            const result = await interfaceService.deleteInterface(interfaceId, userId);
            
            await auditService.logActivity({
                userId: userId,
                action: 'interface_deleted',
                resource: 'interface',
                resourceId: interfaceId,
                details: `Interface deleted: ${result.name}`,
                metadata: {
                    interfaceName: result.name,
                    deletedBy: userEmail
                }
            });
            
            res.json({
                success: true,
                message: 'Interface deleted successfully'
            });
            
        } catch (error) {
            console.error('❌ Delete interface error:', error);
            
            res.status(500).json({
                success: false,
                error: error.message || 'Failed to delete interface'
            });
        }
    }
    
    /**
     * Get interface statistics
     */
    async getInterfaceStats(req, res) {
        try {
            const userId = req.session.user.id;
            const interfaceId = req.params.id;
            
            const interfaceRecord = await interfaceService.getInterfaceById(interfaceId, userId);
            
            if (!interfaceRecord) {
                return res.status(404).json({
                    success: false,
                    error: 'Interface not found'
                });
            }
            
            res.json({
                success: true,
                data: {
                    interfaceId: interfaceRecord.id,
                    name: interfaceRecord.name,
                    status: interfaceRecord.status,
                    sourceType: interfaceRecord.source_type,
                    targetType: interfaceRecord.target_type,
                    sourceConnectivity: interfaceRecord.source_connectivity,
                    targetConnectivity: interfaceRecord.target_connectivity,
                    stats: {
                        totalProcessed: interfaceRecord.total_processed || 0,
                        successful: interfaceRecord.successful_processed || 0,
                        failed: interfaceRecord.failed_processed || 0,
                        successRate: interfaceRecord.total_processed > 0 
                            ? ((interfaceRecord.successful_processed / interfaceRecord.total_processed) * 100).toFixed(2) + '%'
                            : '0%',
                        lastProcessed: interfaceRecord.last_processed_at || null
                    }
                }
            });
            
        } catch (error) {
            console.error('❌ Get stats error:', error);
            
            res.status(500).json({
                success: false,
                error: error.message || 'Failed to get statistics'
            });
        }
    }

    /**
     * ✅ NEW: Check for duplicate interface names
     * POST /api/wizard/check-duplicate
     * 
     * Request body: { name: "Interface Name" }
     * Response: { success: boolean, isDuplicate: boolean, message?: string }
     */
    async checkDuplicateName(req, res) {
        try {
            const { name } = req.body;
            const userId = req.session.user.id;

            console.log('\n=== CHECK DUPLICATE NAME ===');
            console.log('🔍 Checking name:', name);
            console.log('🔍 For user:', req.session.user.email);

            if (!name || typeof name !== 'string' || !name.trim()) {
                return res.status(400).json({
                    success: false,
                    error: 'Interface name is required'
                });
            }

            const trimmedName = name.trim();
            
            // Check if interface with this name exists for this user
            const existingInterface = await interfaceService.findInterfaceByName(trimmedName, userId);
            
            const isDuplicate = !!existingInterface;
            
            console.log('🔍 Duplicate check result:', {
                name: trimmedName,
                isDuplicate: isDuplicate,
                existingId: existingInterface?.id || null
            });

            res.json({
                success: true,
                isDuplicate: isDuplicate,
                message: isDuplicate 
                    ? `Interface name "${trimmedName}" already exists. Please choose a different name.`
                    : `Interface name "${trimmedName}" is available.`
            });

        } catch (error) {
            console.error('❌ Check duplicate name error:', error);
            
            // For duplicate check failures, we should allow progression
            // but log the issue for monitoring
            res.json({
                success: true,
                isDuplicate: false,
                message: 'Unable to verify name uniqueness. Please ensure the name is unique.',
                warning: 'Duplicate check service temporarily unavailable'
            });
        }
    }

    // Fix for wizardController.js completeWizard method
// Replace the existing completeWizard method around line 291

async completeWizard(req, res) {
    try {
        const userId = req.session.user.id;
        const wizardData = req.body;

        console.log('\n=== COMPLETE WIZARD ===');
        console.log('🎯 User:', req.session.user.email);
        console.log('🎯 Interface name:', wizardData.name);
        
        // Map wizard format to interface format
        const mappedWizardData = this.mapWizardToInterfaceData(wizardData);
        
        console.log('🔧 Source mapping:', `${wizardData.sourceType} → format: ${mappedWizardData.sourceType}, connectivity: ${mappedWizardData.sourceConnectivity}`);
        console.log('🔧 Target mapping:', `${wizardData.targetType} → format: ${mappedWizardData.targetType}, connectivity: ${mappedWizardData.targetConnectivity}`);
        console.log('🔄 Complete wizard mapping:', {
            source: `${mappedWizardData.sourceType} → ${mappedWizardData.sourceType}/${mappedWizardData.sourceConnectivity}`,
            target: `${mappedWizardData.targetType} → ${mappedWizardData.targetType}/${mappedWizardData.targetConnectivity}`,
            status: mappedWizardData.status
        });

        // FIXED: Use createInterface instead of completeWizard
        const result = await interfaceService.createInterface(mappedWizardData, userId);
        
        // Log completion
        await auditService.logEvent({
            userId: userId,
            sessionId: req.sessionID,
            action: 'WIZARD_COMPLETED',
            entityType: 'Interface',
            entityId: result.interfaceId,
            newValues: {
                name: mappedWizardData.name,
                messageType: mappedWizardData.messageType,
                sourceType: mappedWizardData.sourceType,
                targetType: mappedWizardData.targetType,
                sourceConnectivity: mappedWizardData.sourceConnectivity,
                targetConnectivity: mappedWizardData.targetConnectivity,
                status: mappedWizardData.status,
                mappingCount: mappedWizardData.mappings?.length || 0
            },
            metadata: {
                source: 'wizard',
                step: 'complete',
                activatedImmediately: true,
                architecture: 'format_connectivity_v2'
            },
            ipAddress: req.clientIP || req.ip,
            userAgent: req.get('user-agent'),
            requestId: req.requestId || uuidv4(),
            result: 'success',
            riskLevel: 'medium' // Higher risk for immediate activation
        });
        
        console.log(`✅ Wizard completed: ${result.interfaceId} with status: ${mappedWizardData.status}`);
        
        res.json({
            success: true,
            data: {
                interfaceId: result.interfaceId,
                interface: result.interface,
                message: 'Wizard completed successfully! Interface is now active.'
            }
        });
        
    } catch (error) {
        console.error('❌ Wizard completion error:', error);
        
        res.status(500).json({
            success: false,
            error: error.message || 'Failed to complete wizard'
        });
    }
    }

    // Add this method to your wizardController.js class

/**
 * Map wizard data format to interface data format
 */
// Fix for wizardController.js - update the mapWizardToInterfaceData method

mapWizardToInterfaceData(wizardData) {
    console.log('Raw wizard data received:', wizardData);
    
    // The wizard sends data in a nested structure, extract the actual step data
    let actualData = wizardData;
    
    // Check if data is nested in wizardData property
    if (wizardData.wizardData) {
        actualData = wizardData.wizardData;
    }
    
    // Check if data is in step format
    if (wizardData.step1Data || wizardData.steps) {
        actualData = {
            name: wizardData.step1Data?.name || wizardData.name,
            sourceType: wizardData.step1Data?.sourceType || wizardData.sourceType,
            targetType: wizardData.step1Data?.targetType || wizardData.targetType,
            sourceConnectivity: wizardData.step1Data?.sourceConnectivity || wizardData.sourceConnectivity,
            targetConnectivity: wizardData.step1Data?.targetConnectivity || wizardData.targetConnectivity,
            messageType: wizardData.step2Data?.messageType || wizardData.messageType,
            mappings: wizardData.step4Data?.mappings || wizardData.mappings
        };
    }
    
    // Extract values with multiple fallback options
    const name = actualData.name || actualData.interfaceName || 'Untitled Interface';
    const description = actualData.description || actualData.interfaceDescription || '';
    const sourceType = actualData.sourceType || 'hl7';
    const sourceConnectivity = actualData.sourceConnectivity || 'tcp';
    const targetType = actualData.targetType || 'fhir';
    const targetConnectivity = actualData.targetConnectivity || 'http';
    
    console.log('Mapped wizard data:', {
        name,
        sourceType,
        sourceConnectivity,
        targetType,
        targetConnectivity
    });
    
    return {
        name: name,
        description: description,
        messageType: actualData.messageType || 'ADT^A01',
        sourceType: sourceType,
        sourceConnectivity: sourceConnectivity,
        targetType: targetType,
        targetConnectivity: targetConnectivity,
        sourceConfig: this.prepareSourceConfig(actualData),
        targetConfig: this.prepareTargetConfig(actualData),
        processingRules: this.prepareProcessingRules(actualData),
        transformationMapping: this.prepareTransformationMapping(actualData),
        status: actualData.status || 'active',
        mappings: actualData.mappings || []
    };
}

// Add this method to fix Step 5 summary elements
ensureSummaryElements() {
    const step5Content = document.querySelector('#step5 .step-content');
    if (!step5Content) return;
    
    // Check if summary elements already exist
    if (document.getElementById('summaryName')) return;
    
    // Create the missing summary elements
    const summaryHTML = `
        <div class="summary-section">
            <h4>Interface Summary</h4>
            <div class="summary-grid">
                <div class="summary-item">
                    <label>Interface Name:</label>
                    <span id="summaryName">-</span>
                </div>
                <div class="summary-item">
                    <label>Interface Type:</label>
                    <span id="summaryType">-</span>
                </div>
                <div class="summary-item">
                    <label>Message Type:</label>
                    <span id="summaryMessage">-</span>
                </div>
                <div class="summary-item">
                    <label>Segments Found:</label>
                    <span id="summaryZSegments">-</span>
                </div>
            </div>
        </div>
        <div class="completion-actions">
            <button type="button" class="btn btn-success" id="completeWizardBtn">
                Create Interface
            </button>
        </div>
    `;
    
    step5Content.innerHTML = summaryHTML;
    
    // Add event listener for completion button
    document.getElementById('completeWizardBtn')?.addEventListener('click', () => {
        this.completeWizard();
    });
    
    console.log('Summary elements created successfully');
}

// Update the complete wizard method to collect data properly
async completeWizard() {
    try {
        // Ensure summary elements exist
        this.ensureSummaryElements();
        
        // Collect wizard data from the global wizard instance
        const wizardData = this.collectWizardData();
        
        console.log('Completing wizard with data:', wizardData);
        
        const response = await fetch('/api/wizard/complete', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(wizardData)
        });
        
        const result = await response.json();
        
        if (result.success) {
            alert('Interface created successfully!');
            // Close wizard and refresh interface list
            window.location.reload();
        } else {
            alert('Failed to create interface: ' + (result.error || 'Unknown error'));
        }
        
    } catch (error) {
        console.error('Wizard completion error:', error);
        alert('Error completing wizard: ' + error.message);
    }
}

// Add method to collect current wizard data
collectWizardData() {
    // Try to get data from global wizard instance
    if (window.wizard && window.wizard.data) {
        return window.wizard.data;
    }
    
    // Fallback: collect from form elements
    return {
        name: document.getElementById('interfaceName')?.value || 'Test Interface',
        sourceType: document.getElementById('sourceFormat')?.value || 'hl7',
        targetType: document.getElementById('targetFormat')?.value || 'fhir', 
        sourceConnectivity: document.getElementById('sourceConnectivity')?.value || 'tcp',
        targetConnectivity: document.getElementById('targetConnectivity')?.value || 'http',
        messageType: 'ADT^A01' // Get from parsed HL7 data if available
    };
}

/**
 * Extract source type from wizard data
 */
extractSourceType(wizardData) {
    if (wizardData.sourceType) return wizardData.sourceType;
    if (wizardData.source?.type) return wizardData.source.type;
    if (wizardData.step1Data?.sourceType) return wizardData.step1Data.sourceType;
    return 'hl7'; // default
}

/**
 * Extract source connectivity from wizard data
 */
extractSourceConnectivity(wizardData) {
    if (wizardData.sourceConnectivity) return wizardData.sourceConnectivity;
    if (wizardData.source?.connectivity) return wizardData.source.connectivity;
    if (wizardData.step1Data?.sourceConnectivity) return wizardData.step1Data.sourceConnectivity;
    
    // Map from legacy format
    const sourceType = this.extractSourceType(wizardData);
    if (sourceType === 'hl7v2' || sourceType === 'hl7') return 'tcp';
    if (sourceType === 'fhir') return 'http';
    
    return 'tcp'; // default
}

/**
 * Extract target type from wizard data
 */
extractTargetType(wizardData) {
    if (wizardData.targetType) return wizardData.targetType;
    if (wizardData.target?.type) return wizardData.target.type;
    if (wizardData.step1Data?.targetType) return wizardData.step1Data.targetType;
    return 'fhir'; // default
}

/**
 * Extract target connectivity from wizard data
 */
extractTargetConnectivity(wizardData) {
    if (wizardData.targetConnectivity) return wizardData.targetConnectivity;
    if (wizardData.target?.connectivity) return wizardData.target.connectivity;
    if (wizardData.step1Data?.targetConnectivity) return wizardData.step1Data.targetConnectivity;
    
    // Map from legacy format
    const targetType = this.extractTargetType(wizardData);
    if (targetType === 'fhir') return 'http';
    if (targetType === 'hl7v2' || targetType === 'hl7') return 'tcp';
    
    return 'http'; // default
}

/**
 * Prepare source configuration
 */
prepareSourceConfig(wizardData) {
    const sourceType = this.extractSourceType(wizardData);
    const sourceConnectivity = this.extractSourceConnectivity(wizardData);
    
    let config = {
        type: sourceType,
        connectivity: sourceConnectivity
    };
    
    // Add connectivity-specific config
    if (sourceConnectivity === 'tcp') {
        config.host = wizardData.sourceHost || 'localhost';
        config.port = wizardData.sourcePort || 8080;
    } else if (sourceConnectivity === 'http') {
        config.endpoint = wizardData.sourceEndpoint || '';
        config.method = wizardData.sourceMethod || 'POST';
    }
    
    return config;
}

/**
 * Prepare target configuration
 */
prepareTargetConfig(wizardData) {
    const targetType = this.extractTargetType(wizardData);
    const targetConnectivity = this.extractTargetConnectivity(wizardData);
    
    let config = {
        type: targetType,
        connectivity: targetConnectivity
    };
    
    // Add connectivity-specific config
    if (targetConnectivity === 'http') {
        config.endpoint = wizardData.targetEndpoint || '';
        config.method = wizardData.targetMethod || 'POST';
        config.headers = wizardData.targetHeaders || {};
    } else if (targetConnectivity === 'tcp') {
        config.host = wizardData.targetHost || 'localhost';
        config.port = wizardData.targetPort || 8080;
    }
    
    return config;
}

/**
 * Prepare processing rules
 */
prepareProcessingRules(wizardData) {
    return {
        validateInput: wizardData.validateInput !== false,
        validateOutput: wizardData.validateOutput !== false,
        retryFailures: wizardData.retryFailures !== false,
        maxRetries: wizardData.maxRetries || 3,
        timeout: wizardData.timeout || 30000,
        batchSize: wizardData.batchSize || 1
    };
}

/**
 * Prepare transformation mapping
 */
prepareTransformationMapping(wizardData) {
    return {
        mappings: wizardData.mappings || [],
        rules: wizardData.transformationRules || [],
        profile: wizardData.fhirProfile || 'base',
        version: wizardData.fhirVersion || 'R4'
    };
}

// Updated completeWizard method
async completeWizard(req, res) {
    try {
        const userId = req.session.user.id;
        const wizardData = req.body;

        console.log('\n=== COMPLETE WIZARD ===');
        console.log('🎯 User:', req.session.user.email);
        console.log('🎯 Raw wizard data:', JSON.stringify(wizardData, null, 2));
        
        // Map wizard format to interface format
        const mappedWizardData = this.mapWizardToInterfaceData(wizardData);
        
        console.log('🎯 Interface name:', mappedWizardData.name);
        console.log('🔧 Source mapping:', `${wizardData.sourceType} → format: ${mappedWizardData.sourceType}, connectivity: ${mappedWizardData.sourceConnectivity}`);
        console.log('🔧 Target mapping:', `${wizardData.targetType} → format: ${mappedWizardData.targetType}, connectivity: ${mappedWizardData.targetConnectivity}`);
        
        // Create interface using existing service method
        const result = await interfaceService.createInterface(mappedWizardData, userId);
        
        // Log completion
        if (auditService && typeof auditService.logEvent === 'function') {
            await auditService.logEvent({
                userId: userId,
                sessionId: req.sessionID,
                action: 'WIZARD_COMPLETED',
                entityType: 'Interface',
                entityId: result.interfaceId,
                newValues: {
                    name: mappedWizardData.name,
                    messageType: mappedWizardData.messageType,
                    sourceType: mappedWizardData.sourceType,
                    targetType: mappedWizardData.targetType,
                    status: mappedWizardData.status
                },
                result: 'success'
            });
        }
        
        console.log(`✅ Wizard completed: ${result.interfaceId} with status: ${mappedWizardData.status}`);
        
        res.json({
            success: true,
            data: {
                interfaceId: result.interfaceId,
                interface: result.interface,
                message: 'Wizard completed successfully! Interface is now active.'
            }
        });
        
    } catch (error) {
        console.error('❌ Wizard completion error:', error);
        
        res.status(500).json({
            success: false,
            error: error.message || 'Failed to complete wizard'
        });
    }
}
}

module.exports = new WizardController();