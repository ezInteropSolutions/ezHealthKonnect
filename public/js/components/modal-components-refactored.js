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

        // ✅ REFACTORED: Using shared InterfaceConfigComponents
        container.innerHTML = `
            <!-- Edit Interface Modal (Refactored) -->
            <div class="modal-overlay" id="editModal">
                <div class="modal-content large">
                    <div class="modal-header">
                        <h3 class="modal-title" id="editTitle">Edit Interface Configuration</h3>
                        <button class="modal-close" onclick="closeEditModal()">&times;</button>
                    </div>
                    <div class="modal-body">
                        <form id="editInterfaceForm" onsubmit="handleEditInterface(event)">
                            <input type="hidden" id="editInterfaceId" name="id">

                            <!-- Basic Information Section -->
                            <div class="config-section">
                                <h4 class="section-title">📋 Basic Information</h4>
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
                                        <label for="editStatus" class="form-label">Status</label>
                                        <select id="editStatus" class="form-control" name="status">
                                            <option value="inactive">Inactive</option>
                                            <option value="testing">Testing</option>
                                            <option value="active">Active</option>
                                            <option value="configured">Configured</option>
                                        </select>
                                    </div>
                                </div>
                            </div>

                            <!-- Source Configuration Section (Uses Shared Components) -->
                            <div class="config-section">
                                <h4 class="section-title">📥 Source Configuration</h4>

                                <div class="form-row">
                                    <!-- Source Type Selector (rendered by shared component) -->
                                    <div id="editSourceTypeContainer"></div>

                                    <!-- Source Connectivity Selector (rendered by shared component) -->
                                    <div id="editSourceConnectivityContainer"></div>
                                </div>

                                <!-- Dynamic Source Configuration Panel (rendered by shared component) -->
                                <div id="editSourceConfigPanel" class="config-panel"></div>
                            </div>

                            <!-- Target Configuration Section -->
                            <div class="config-section">
                                <h4 class="section-title">🎯 Target Configuration</h4>

                                <div class="form-group">
                                    <label for="editTargetType" class="form-label">Target Type</label>
                                    <select id="editTargetType" class="form-control" name="targetType">
                                        <option value="fhir">FHIR Server</option>
                                        <option value="hl7v2">HL7 v2.x System</option>
                                        <option value="database">Database</option>
                                        <option value="file">File System</option>
                                        <option value="http">HTTP API</option>
                                    </select>
                                </div>

                                <div class="form-group">
                                    <label for="editTargetEndpoint" class="form-label">Target Endpoint</label>
                                    <input type="text" id="editTargetEndpoint" class="form-control"
                                           name="targetEndpoint" placeholder="http://localhost:8080/fhir">
                                    <div class="form-hint">Full URL to target server</div>
                                </div>
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

        // Extract connectivity from V30 JSONB structure or fallback to string
        let sourceConnectivityValue = interfaceData.sourceConnectivity;
        let sourceConfigData = interfaceData.sourceConfig || {};

        // Handle V30 migration: source_connectivity might be JSONB {type, config}
        if (typeof sourceConnectivityValue === 'object' && sourceConnectivityValue !== null) {
            console.log('🔄 Detected V30 JSONB connectivity structure:', sourceConnectivityValue);
            const connectivityObj = sourceConnectivityValue;
            sourceConnectivityValue = connectivityObj.type || 'tcp';
            // Merge config from connectivity object
            sourceConfigData = { ...connectivityObj.config, ...sourceConfigData };
        }

        // Source Type - use shared component
        const sourceTypeContainer = document.getElementById('editSourceTypeContainer');
        if (sourceTypeContainer) {
            sourceTypeContainer.innerHTML = InterfaceConfigComponents.getSourceTypeSelector(
                interfaceData.sourceType || 'hl7v2',
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
        updateEditSourceConfigPanel({
            ...interfaceData,
            sourceConnectivity: sourceConnectivityValue,
            sourceConfig: sourceConfigData
        });

        // Target fields
        document.getElementById('editTargetType').value = interfaceData.targetType || 'fhir';
        document.getElementById('editTargetEndpoint').value = interfaceData.targetEndpoint ||
                                                               interfaceData.targetConfig?.endpoint || '';

        // Attach event listeners
        attachEditModalListeners();
    };

    /**
     * Update Edit Source Config Panel
     */
    function updateEditSourceConfigPanel(interfaceData) {
        const sourceConfigPanel = document.getElementById('editSourceConfigPanel');
        if (!sourceConfigPanel) return;

        const sourceType = document.getElementById('editsourceType')?.value || interfaceData.sourceType || 'hl7v2';
        const sourceConnectivity = document.getElementById('editsourceConnectivity')?.value || interfaceData.sourceConnectivity || 'tcp';
        const sourceConfig = interfaceData.sourceConfig || {};

        console.log('🔄 Updating edit source config panel:', { sourceType, sourceConnectivity });

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

        console.log('✅ Edit modal listeners attached');
    }

    // Run on load
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', loadModalComponents);
    } else {
        loadModalComponents();
    }

    console.log('✅ Modal components loader initialized (REFACTORED VERSION)');
})();
