// js/components/modal-components-refactored.js - Modal Components Loader (Refactored)
// Uses shared InterfaceConfigComponents for consistency

(function() {
    'use strict';

    function loadModalComponents() {
        loadCreateModal();
        loadEditModal();
        loadDetailsModal();
    }

    function loadCreateModal() {
        const container = document.getElementById('create-modal-container');
        if (!container) {
            console.warn('⚠️ Create modal container not found');
            return;
        }
        console.log('✅ Loading create modal...');

        // Simple create modal - redirects to wizard for full configuration
        container.innerHTML = `
            <!-- Compact Create Interface Modal -->
            <div class="modal-overlay" id="createModal">
                <div class="modal-content">
                    <div class="modal-header">
                        <h3 class="modal-title">Create New Interface</h3>
                        <button class="modal-close" onclick="closeCreateModal()">&times;</button>
                    </div>
                    <div class="modal-body">
                        <form id="createInterfaceForm">
                            <div class="form-group">
                                <label for="interfaceName">Interface Name</label>
                                <input type="text" id="interfaceName" name="name" required
                                       placeholder="e.g., ADT Patient Admissions">
                            </div>

                            <div class="form-group">
                                <label for="interfaceDescription">Description</label>
                                <textarea id="interfaceDescription" name="description"
                                          placeholder="Brief description of this interface"></textarea>
                            </div>

                            <div class="form-row">
                                <div class="form-group">
                                    <label for="sourceType">Source Type</label>
                                    <select id="sourceType" name="sourceType" required>
                                        <option value="">Select source...</option>
                                        <option value="file">File</option>
                                        <option value="tcp">TCP</option>
                                        <option value="http">HTTP</option>
                                        <option value="database">Database</option>
                                    </select>
                                </div>

                                <div class="form-group">
                                    <label for="targetType">Target Type</label>
                                    <select id="targetType" name="targetType" required>
                                        <option value="">Select target...</option>
                                        <option value="file">File</option>
                                        <option value="tcp">TCP</option>
                                        <option value="http">HTTP</option>
                                        <option value="database">Database</option>
                                        <option value="fhir">FHIR</option>
                                    </select>
                                </div>
                            </div>
                        </form>
                    </div>
                    <div class="modal-footer">
                        <button type="button" class="modal-btn secondary" onclick="closeCreateModal()">Cancel</button>
                        <button type="submit" class="modal-btn primary" form="createInterfaceForm">Create Interface</button>
                    </div>
                </div>
            </div>
        `;
    }

    function loadEditModal() {
        const container = document.getElementById('edit-modal-container');
        if (!container) {
            console.warn('⚠️ Edit modal container not found');
            return;
        }
        console.log('✅ Loading edit modal (REFACTORED with shared components)...');

        // ✅ REFACTORED: Using shared InterfaceConfigComponents with tabs and maximize
        container.innerHTML = `
            <!-- Edit Interface Modal (Refactored with Tabs) -->
            <div class="modal-overlay" id="editModal">
                <div class="modal-content large" id="editModalContent">
                    <div class="modal-header">
                        <h3 class="modal-title" id="editTitle">Edit Interface Configuration</h3>
                        <div class="modal-controls">
                            <button class="modal-control-btn" id="editModalMaximize" onclick="toggleEditModalMaximize()" title="Maximize">
                                <span id="editMaximizeIcon">⛶</span>
                            </button>
                            <button class="modal-close" onclick="closeEditModal()">&times;</button>
                        </div>
                    </div>

                    <!-- Tab Navigation -->
                    <div class="modal-tabs">
                        <button class="modal-tab active" data-tab="basic" onclick="switchEditTab('basic')">
                            <span class="tab-icon">&#9776;</span> Basic
                        </button>
                        <button class="modal-tab" data-tab="source" onclick="switchEditTab('source')">
                            <span class="tab-icon">&#8594;</span> Source
                        </button>
                        <button class="modal-tab" data-tab="target" onclick="switchEditTab('target')">
                            <span class="tab-icon">&#9654;</span> Target
                        </button>
                        <button class="modal-tab" data-tab="deployment" onclick="switchEditTab('deployment')">
                            <span class="tab-icon">&#9881;</span> Deploy
                        </button>
                    </div>

                    <div class="modal-body">
                        <form id="editInterfaceForm" onsubmit="handleEditInterface(event)">
                            <input type="hidden" id="editInterfaceId" name="id">

                            <!-- Tab 1: Basic Information -->
                            <div class="tab-content active" id="editTabBasic">
                                <div class="config-section">
                                    <div class="form-group">
                                        <label for="editInterfaceName" class="form-label required">Interface Name</label>
                                        <input type="text" id="editInterfaceName" class="form-control" name="name" required>
                                    </div>

                                    <div class="form-group">
                                        <label for="editInterfaceDescription" class="form-label">Description</label>
                                        <textarea id="editInterfaceDescription" class="form-control" name="description" rows="3"></textarea>
                                    </div>

                                    <div class="form-row">
                                        <div class="form-group">
                                            <label for="editStatus" class="form-label">Interface Status</label>
                                            <select id="editStatus" class="form-control" name="status">
                                                <option value="draft">📝 Draft - Initial configuration</option>
                                                <option value="configured">⚙️ Configured - Setup complete</option>
                                                <option value="testing">🧪 Testing - Under validation</option>
                                                <option value="active">✅ Active - Production ready</option>
                                                <option value="inactive">⏸️ Inactive - Temporarily disabled</option>
                                                <option value="error">❌ Error - Requires attention</option>
                                            </select>
                                            <small style="color: #64748b; display: block; margin-top: 0.5rem;">
                                                Lifecycle: Draft → Configured → Testing → Active
                                            </small>
                                        </div>
                                    </div>

                                    <!-- Logging & Troubleshooting Section -->
                                    <div style="margin-top: 2rem; padding-top: 1.5rem; border-top: 2px solid #e2e8f0;">
                                        <h4 style="color: #1e3a8a; font-size: 1.1rem; margin-bottom: 1rem; font-weight: 600;">
                                            <i class="fas fa-file-alt"></i> Logging & Troubleshooting
                                        </h4>

                                        <!-- Log Level Selector -->
                                        <div class="form-group">
                                            <div style="background: linear-gradient(to right, #f0f9ff, #f5f3ff); border-left: 3px solid #60a5fa; padding: 14px; border-radius: 6px;">
                                                <label for="editLogLevel" style="font-weight: 600; color: #1e3a8a; font-size: 0.95rem; display: block; margin-bottom: 10px;">
                                                    📝 Log Level
                                                </label>
                                                <div style="display: grid; grid-template-columns: repeat(5, 1fr); gap: 6px;" id="logLevelGroup">
                                                    <label class="log-level-option" data-level="debug" style="cursor:pointer;text-align:center;">
                                                        <input type="radio" name="log_level" id="editLogLevel" value="debug" style="display:none;">
                                                        <div class="log-level-chip" style="padding:6px 4px;border-radius:6px;border:2px solid #e2e8f0;font-size:0.78rem;font-weight:600;transition:all 0.15s;">
                                                            🔬 Debug
                                                        </div>
                                                        <div style="font-size:0.7rem;color:#94a3b8;margin-top:3px;">All logs</div>
                                                    </label>
                                                    <label class="log-level-option" data-level="info" style="cursor:pointer;text-align:center;">
                                                        <input type="radio" name="log_level" value="info" style="display:none;">
                                                        <div class="log-level-chip" style="padding:6px 4px;border-radius:6px;border:2px solid #e2e8f0;font-size:0.78rem;font-weight:600;transition:all 0.15s;">
                                                            ℹ️ Info
                                                        </div>
                                                        <div style="font-size:0.7rem;color:#94a3b8;margin-top:3px;">Info+</div>
                                                    </label>
                                                    <label class="log-level-option" data-level="warning" style="cursor:pointer;text-align:center;">
                                                        <input type="radio" name="log_level" value="warning" style="display:none;">
                                                        <div class="log-level-chip" style="padding:6px 4px;border-radius:6px;border:2px solid #e2e8f0;font-size:0.78rem;font-weight:600;transition:all 0.15s;">
                                                            ⚠️ Warn
                                                        </div>
                                                        <div style="font-size:0.7rem;color:#94a3b8;margin-top:3px;">Warn+</div>
                                                    </label>
                                                    <label class="log-level-option" data-level="error" style="cursor:pointer;text-align:center;">
                                                        <input type="radio" name="log_level" value="error" style="display:none;">
                                                        <div class="log-level-chip" style="padding:6px 4px;border-radius:6px;border:2px solid #e2e8f0;font-size:0.78rem;font-weight:600;transition:all 0.15s;">
                                                            ❌ Error
                                                        </div>
                                                        <div style="font-size:0.7rem;color:#94a3b8;margin-top:3px;">Errors only</div>
                                                    </label>
                                                    <label class="log-level-option" data-level="off" style="cursor:pointer;text-align:center;">
                                                        <input type="radio" name="log_level" value="off" style="display:none;">
                                                        <div class="log-level-chip" style="padding:6px 4px;border-radius:6px;border:2px solid #e2e8f0;font-size:0.78rem;font-weight:600;transition:all 0.15s;">
                                                            🔕 Off
                                                        </div>
                                                        <div style="font-size:0.7rem;color:#94a3b8;margin-top:3px;">No logs</div>
                                                    </label>
                                                </div>
                                                <div id="logLevelHint" style="font-size:0.82rem;color:#6b7280;margin-top:10px;line-height:1.4;">
                                                    🔬 <strong>Debug</strong> — captures every step for full visibility. Uses more storage.
                                                </div>
                                                <!-- hidden checkbox kept for backward-compat with save logic -->
                                                <input type="checkbox" id="editDebugLogging" name="debug_logging" checked style="display:none;">
                                            </div>
                                        </div>

                                        <!-- Log Retention Period & Error Retention -->
                                        <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 16px; margin-top: 16px;">
                                            <div class="form-group" style="margin: 0;">
                                                <label for="editLogRetention" class="form-label" style="font-weight: 600; color: #1e3a8a; margin-bottom: 8px;">
                                                    🗑️ Log Retention Period
                                                </label>
                                                <div style="display: flex; align-items: center; gap: 8px;">
                                                    <input type="number" id="editLogRetention" class="form-control" name="log_retention_days"
                                                           value="30" min="1" max="365"
                                                           style="max-width: 100px; text-align: center; font-weight: 600;">
                                                    <span style="color: #6b7280; font-size: 0.9rem;">days</span>
                                                </div>
                                                <small style="color: #64748b; display: block; margin-top: 6px;">
                                                    Debug/info logs auto-deleted after this period
                                                </small>
                                            </div>

                                            <div class="form-group" style="margin: 0;">
                                                <label class="form-label" style="font-weight: 600; color: #1e3a8a; margin-bottom: 8px;">
                                                    ♾️ Error Log Retention
                                                </label>
                                                <div style="background: #f0f9ff; border: 1px solid #bfdbfe; padding: 10px 12px; border-radius: 6px;">
                                                    <label style="display: flex; align-items: center; cursor: pointer; margin: 0;">
                                                        <input type="checkbox" id="editRetainErrors" name="retain_error_logs_forever"
                                                               checked style="margin-right: 8px; width: 18px; height: 18px; cursor: pointer; accent-color: #0369a1;">
                                                        <span style="font-size: 0.9rem; color: #1e3a8a; font-weight: 500;">Keep errors forever</span>
                                                    </label>
                                                </div>
                                                <small style="color: #64748b; display: block; margin-top: 6px;">
                                                    ✅ Recommended for compliance
                                                </small>
                                            </div>
                                        </div>

                                        <!-- Info Panel -->
                                        <div style="background: #f9fafb; border: 1px solid #e5e7eb; padding: 12px; border-radius: 6px; margin-top: 16px;">
                                            <div style="font-size: 0.875rem; color: #374151; line-height: 1.6;">
                                                <strong style="color: #1e3a8a;">Retention Policy Summary:</strong><br>
                                                • <strong>Debug/Info logs:</strong> Deleted after <span id="editRetentionSummary" style="color: #0369a1; font-weight: 600;">30 days</span><br>
                                                • <strong>Error/Warning logs:</strong> <span id="editErrorRetentionSummary" style="color: #059669; font-weight: 600;">Kept forever</span><br>
                                                • <strong>Audit logs:</strong> <span style="color: #059669; font-weight: 600;">Always retained (HIPAA compliance)</span>
                                            </div>
                                        </div>
                                    </div>
                                </div>

                                <!-- FHIR Validation Policy Section -->
                                <div class="form-group" style="margin-top: 16px;">
                                    <label class="form-label" for="fhirValidationPolicy" style="font-weight:600;color:#374151;font-size:13px;margin-bottom:4px;display:block;">
                                        FHIR Validation Policy
                                    </label>
                                    <select id="fhirValidationPolicy" name="fhirValidationPolicy"
                                            style="width:100%;padding:7px 10px;border:1px solid #d1d5db;border-radius:6px;font-size:13px;color:#1f2937;background:#fff;cursor:pointer;">
                                        <option value="proceed">Proceed — omit missing field and continue (default)</option>
                                        <option value="warn">Warn — deliver with OperationOutcome warning in bundle</option>
                                        <option value="reject">Reject — fail message, route to dead-letter queue</option>
                                        <option value="queue_review">Queue for Review — hold for manual operator correction</option>
                                    </select>
                                </div>
                            </div>

                            <!-- Tab 2: Source Configuration -->
                            <div class="tab-content" id="editTabSource">
                                <div class="config-section">
                                    <div class="form-row">
                                        <!-- Source Type Selector (rendered by shared component) -->
                                        <div id="editSourceTypeContainer"></div>

                                        <!-- Source Connectivity Selector (rendered by shared component) -->
                                        <div id="editSourceConnectivityContainer"></div>
                                    </div>

                                    <!-- Dynamic Source Configuration Panel (rendered by shared component) -->
                                    <div id="editSourceConfigPanel" class="config-panel"></div>
                                </div>
                            </div>

                            <!-- Tab 3: Target Configuration -->
                            <div class="tab-content" id="editTabTarget">
                                <div class="config-section">
                                    <!-- Target Connectivity Selector (rendered by shared component) -->
                                    <div id="editTargetConnectivityContainer"></div>

                                    <!-- Dynamic Target Configuration Panel (rendered by shared component) -->
                                    <div id="editTargetConfigPanel" class="config-panel"></div>
                                </div>
                            </div>

                            <!-- Tab 4: Deployment Settings -->
                            <div class="tab-content" id="editTabDeployment">
                                <div id="editDeploymentSettingsContainer"></div>
                            </div>

                        </form>
                    </div>
                    <div class="modal-footer">
                        <button type="button" class="modal-btn secondary" onclick="closeEditModal()">Cancel</button>
                        <button type="submit" class="modal-btn primary" form="editInterfaceForm">Save Changes</button>
                    </div>
                </div>
            </div>
        `;

        // Setup tab switching and maximize functions
        setupEditModalFunctions();
    }

    function loadDetailsModal() {
        const container = document.getElementById('details-modal-container');
        if (!container) {
            console.warn('⚠️ Details modal container not found');
            return;
        }
        console.log('✅ Loading details modal...');

        container.innerHTML = `
            <!-- Interface Details Modal -->
            <div class="modal-overlay" id="detailsModal">
                <div class="modal-content large">
                    <div class="modal-header">
                        <h3 class="modal-title" id="detailsTitle">Interface Details</h3>
                        <button class="modal-close" onclick="closeDetailsModal()">&times;</button>
                    </div>
                    <div class="modal-body" id="detailsContent">
                        <!-- Content populated dynamically -->
                    </div>
                    <div class="modal-footer">
                        <button type="button" class="modal-btn secondary" onclick="closeDetailsModal()">Close</button>
                    </div>
                </div>
            </div>
        `;
    }

    /**
     * Populate Edit Form with Interface Data
     * Uses shared InterfaceConfigComponents
     */
    window.populateEditForm = function(interfaceData) {
        console.log('📝 Populating edit form with data:', interfaceData);

        // Basic fields
        document.getElementById('editInterfaceId').value = interfaceData.id || '';
        document.getElementById('editInterfaceName').value = interfaceData.name || '';
        document.getElementById('editInterfaceDescription').value = interfaceData.description || '';
        document.getElementById('editStatus').value = interfaceData.status || 'inactive';

        // Logging settings — set log_level selector (default: debug)
        const effectiveLevel = interfaceData.log_level || (interfaceData.debug_logging ? 'debug' : 'debug');
        setLogLevel(effectiveLevel);
        document.getElementById('editLogRetention').value = interfaceData.log_retention_days || 30;
        document.getElementById('editRetainErrors').checked = interfaceData.retain_error_logs_forever !== false;
        // Extract connectivity from V30 JSONB structure or fallback to string
        // Handle both snake_case (from API) and camelCase (from frontend)
        let sourceConnectivityValue = interfaceData.source_connectivity || interfaceData.sourceConnectivity;
        let sourceConfigData = interfaceData.source_config || interfaceData.sourceConfig || {};

        // Handle V30 migration: source_connectivity might be JSONB {type, config}
        if (typeof sourceConnectivityValue === 'object' && sourceConnectivityValue !== null) {
            console.log('🔄 Detected V30 JSONB connectivity structure:', sourceConnectivityValue);
            const connectivityObj = sourceConnectivityValue;
            sourceConnectivityValue = connectivityObj.type || 'tcp';
            // Merge config from connectivity object - this is the actual config to use
            sourceConfigData = { ...connectivityObj.config, ...sourceConfigData };
            console.log('📋 Extracted source config from V30 structure:', sourceConfigData);
        }

        // Source Type - use shared component
        // Handle both snake_case (from API) and camelCase
        const sourceTypeContainer = document.getElementById('editSourceTypeContainer');
        if (sourceTypeContainer) {
            sourceTypeContainer.innerHTML = InterfaceConfigComponents.getSourceTypeSelector(
                interfaceData.source_type || interfaceData.sourceType || 'hl7v2',
                { idPrefix: 'edit', showHint: false }
            );
        }

        // Source Connectivity - use shared component
        const sourceConnectivityContainer = document.getElementById('editSourceConnectivityContainer');
        if (sourceConnectivityContainer) {
            sourceConnectivityContainer.innerHTML = InterfaceConfigComponents.getSourceConnectivitySelector(
                sourceConnectivityValue || 'tcp',
                { idPrefix: 'edit', showHint: false }
            );
        }

        // Source Config Panel - use shared component with extracted data
        const dataForSourcePanel = {
            ...interfaceData,
            sourceConnectivity: sourceConnectivityValue,
            sourceConfig: sourceConfigData,
            sourceType: interfaceData.source_type || interfaceData.sourceType || 'hl7v2'
        };
        console.log('📦 Data being passed to updateEditSourceConfigPanel:', {
            sourceConnectivity: dataForSourcePanel.sourceConnectivity,
            sourceConfig: dataForSourcePanel.sourceConfig,
            sourceType: dataForSourcePanel.sourceType
        });
        updateEditSourceConfigPanel(dataForSourcePanel);

        // Extract target connectivity from V30 JSONB structure or fallback to string
        // Handle both snake_case (from API) and camelCase (from frontend)
        let targetConnectivityValue = interfaceData.target_connectivity || interfaceData.targetConnectivity || 'http';
        let targetConfigData = interfaceData.target_config || interfaceData.targetConfig || {};

        // Handle V30 migration: target_connectivity might be JSONB {type, config}
        if (typeof targetConnectivityValue === 'object' && targetConnectivityValue !== null) {
            console.log('🔄 Detected V30 JSONB target connectivity structure:', targetConnectivityValue);
            const connectivityObj = targetConnectivityValue;
            targetConnectivityValue = connectivityObj.type || 'http';
            // Merge config from connectivity object - this is the actual config to use
            targetConfigData = { ...connectivityObj.config, ...targetConfigData };
            console.log('📋 Extracted target config from V30 structure:', targetConfigData);
        }

        // Target Connectivity - use shared component
        const targetConnectivityContainer = document.getElementById('editTargetConnectivityContainer');
        if (targetConnectivityContainer) {
            targetConnectivityContainer.innerHTML = InterfaceConfigComponents.getTargetConnectivitySelector(
                targetConnectivityValue,
                { idPrefix: 'edit', showHint: false }
            );
        }

        // Target Config Panel - use shared component with extracted data
        updateEditTargetConfigPanel({
            ...interfaceData,
            targetConnectivity: targetConnectivityValue,
            targetConfig: targetConfigData
        });

        // Deployment Settings - use shared component
        const deploymentSettingsContainer = document.getElementById('editDeploymentSettingsContainer');
        if (deploymentSettingsContainer) {
            console.log('🚀 Populating deployment settings with data:', {
                deployment_mode: interfaceData.deployment_mode,
                auto_start: interfaceData.auto_start,
                deployment_delay_seconds: interfaceData.deployment_delay_seconds
            });
            deploymentSettingsContainer.innerHTML = InterfaceConfigComponents.getDeploymentSettingsPanel(
                interfaceData,
                'edit'
            );
            // Initialize event handlers for deployment settings
            InterfaceConfigComponents.initDeploymentSettingsEvents(document, 'edit');
        } else {
            console.warn('⚠️ editDeploymentSettingsContainer not found in DOM');
        }

        // FHIR Validation Policy — set saved value on the select (default: 'proceed')
        const policy = interfaceData.fhir_validation_policy || interfaceData.fhirValidationPolicy || 'proceed';
        const policySelect = document.getElementById('fhirValidationPolicy');
        if (policySelect) policySelect.value = policy;

        // Attach event listeners
        attachEditModalListeners();
    };

    /**
     * Update Edit Source Config Panel
     */
    function updateEditSourceConfigPanel(interfaceData) {
        const sourceConfigPanel = document.getElementById('editSourceConfigPanel');
        if (!sourceConfigPanel) return;

        const sourceType = document.getElementById('editsourceType')?.value || interfaceData.sourceType || interfaceData.source_type || 'hl7v2';
        const sourceConnectivity = document.getElementById('editsourceConnectivity')?.value || interfaceData.sourceConnectivity || 'tcp';
        const sourceConfig = interfaceData.sourceConfig || {};

        console.log('🔄 Updating edit source config panel:', { sourceType, sourceConnectivity, sourceConfig });
        console.log('🔍 DEBUG: Full interfaceData passed to updateEditSourceConfigPanel:', interfaceData);

        sourceConfigPanel.innerHTML = InterfaceConfigComponents.getSourceConfigPanel(
            sourceConnectivity,
            sourceType,
            sourceConfig,
            { idPrefix: 'edit' }
        );

        // Attach event listeners for the new panel
        InterfaceConfigComponents.attachEventListeners(
            sourceConfigPanel,
            'edit',
            {
                onConnectivityChange: () => updateEditSourceConfigPanel(interfaceData),
                onSourceTypeChange: () => updateEditSourceConfigPanel(interfaceData)
            }
        );
    }

    /**
     * Update Edit Target Config Panel
     */
    function updateEditTargetConfigPanel(interfaceData) {
        const targetConfigPanel = document.getElementById('editTargetConfigPanel');
        if (!targetConfigPanel) return;

        const targetConnectivity = document.getElementById('edittargetConnectivity')?.value || interfaceData.targetConnectivity || 'http';
        const targetConfig = interfaceData.targetConfig || {};

        console.log('🔄 Updating edit target config panel:', { targetConnectivity });

        targetConfigPanel.innerHTML = InterfaceConfigComponents.getTargetConfigPanel(
            targetConnectivity,
            null, // targetType not needed
            targetConfig,
            { idPrefix: 'edit' }
        );

        // Attach event listeners for the new panel
        // Use 'edittarget' prefix for auth listeners since auth fields use idPrefix + 'target'
        InterfaceConfigComponents.attachEventListeners(
            targetConfigPanel,
            'edittarget'
        );
    }

    /**
     * Attach Event Listeners to Edit Modal
     */
    function attachEditModalListeners() {
        console.log('🔧 Attaching edit modal event listeners...');

        // Source Type Change
        const sourceTypeSelect = document.getElementById('editsourceType');
        if (sourceTypeSelect) {
            sourceTypeSelect.addEventListener('change', (e) => {
                console.log('🔄 Edit: Source type changed to:', e.target.value);
                updateEditSourceConfigPanel({ sourceType: e.target.value });
            });
        }

        // Source Connectivity Change
        const sourceConnectivitySelect = document.getElementById('editsourceConnectivity');
        if (sourceConnectivitySelect) {
            sourceConnectivitySelect.addEventListener('change', (e) => {
                console.log('🔄 Edit: Source connectivity changed to:', e.target.value);
                updateEditSourceConfigPanel({ sourceConnectivity: e.target.value });
            });
        }

        // Target Connectivity Change
        const targetConnectivitySelect = document.getElementById('edittargetConnectivity');
        if (targetConnectivitySelect) {
            targetConnectivitySelect.addEventListener('change', (e) => {
                console.log('🔄 Edit: Target connectivity changed to:', e.target.value);
                updateEditTargetConfigPanel({ targetConnectivity: e.target.value });
            });
        }

        // FHIR Validation Policy — no extra listener needed; select handles state natively

        console.log('✅ Edit modal listeners attached');
    }

    /**
     * Setup Edit Modal Functions (tabs, maximize)
     */
    function setupEditModalFunctions() {
        // Expose functions globally
        window.switchEditTab = function(tabName) {
            // Update tab buttons
            document.querySelectorAll('.modal-tab').forEach(tab => {
                tab.classList.remove('active');
                if (tab.dataset.tab === tabName) {
                    tab.classList.add('active');
                }
            });

            // Update tab content
            document.querySelectorAll('.tab-content').forEach(content => {
                content.classList.remove('active');
            });

            const tabContent = document.getElementById(`editTab${tabName.charAt(0).toUpperCase() + tabName.slice(1)}`);
            if (tabContent) {
                tabContent.classList.add('active');
            }
        };

        window.toggleEditModalMaximize = function() {
            const modalContent = document.getElementById('editModalContent');
            const icon = document.getElementById('editMaximizeIcon');

            if (modalContent) {
                modalContent.classList.toggle('maximized');
                if (modalContent.classList.contains('maximized')) {
                    icon.textContent = '⧉';
                } else {
                    icon.textContent = '⛶';
                }
            }
        };

        console.log('✅ Edit modal functions setup complete');
    }

    // Run on load
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', loadModalComponents);
    } else {
        loadModalComponents();
    }

    console.log('✅ Modal components loader initialized (REFACTORED VERSION)');
})();
