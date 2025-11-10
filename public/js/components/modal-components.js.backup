// js/components/modal-components.js - Modal Components Loader
// Loads create, edit, and details modals

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
        
        // ✅ PRESERVED: Create modal HTML with original IDs
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
        console.log('✅ Loading edit modal...');
        
        // ✅ ENHANCED: Comprehensive interface configuration modal
        container.innerHTML = `
            <!-- Enhanced Edit Interface Modal -->
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
                                    <label for="editInterfaceName">Interface Name</label>
                                    <input type="text" id="editInterfaceName" name="name" required>
                                </div>

                                <div class="form-group">
                                    <label for="editInterfaceDescription">Description</label>
                                    <textarea id="editInterfaceDescription" name="description" rows="3"></textarea>
                                </div>

                                <div class="form-row">
                                    <div class="form-group">
                                        <label for="editFormat">Message Format</label>
                                        <select id="editFormat" name="format" required>
                                            <option value="HL7">HL7 v2.x</option>
                                            <option value="FHIR">FHIR R4</option>
                                            <option value="JSON">JSON</option>
                                            <option value="XML">XML</option>
                                        </select>
                                    </div>

                                    <div class="form-group">
                                        <label for="editStatus">Status</label>
                                        <select id="editStatus" name="status">
                                            <option value="inactive">Inactive</option>
                                            <option value="testing">Testing</option>
                                            <option value="active">Active</option>
                                            <option value="configured">Configured</option>
                                        </select>
                                    </div>
                                </div>
                            </div>

                            <!-- Source Configuration Section -->
                            <div class="config-section">
                                <h4 class="section-title">📥 Source Configuration</h4>
                                <div class="form-row">
                                    <div class="form-group">
                                        <label for="editSourceType">Source Type</label>
                                        <select id="editSourceType" name="sourceType" onchange="updateSourceFields()" required>
                                            <option value="file">File</option>
                                            <option value="tcp">TCP Listener</option>
                                            <option value="http">HTTP Receiver</option>
                                            <option value="database">Database Poller</option>
                                            <option value="mllp">MLLP (HL7)</option>
                                        </select>
                                    </div>

                                    <div class="form-group">
                                        <label for="editSourceConnectivity">Source Connectivity</label>
                                        <select id="editSourceConnectivity" name="sourceConnectivity" required>
                                            <option value="inbound">Inbound (Receive)</option>
                                            <option value="outbound">Outbound (Send)</option>
                                            <option value="polling">Polling</option>
                                        </select>
                                    </div>
                                </div>

                                <!-- Dynamic Source Configuration -->
                                <div id="sourceConfigFields">
                                    <div class="form-row" id="sourcePortConfig">
                                        <div class="form-group">
                                            <label for="editSourceHost">Source Host/IP</label>
                                            <input type="text" id="editSourceHost" name="sourceHost" placeholder="localhost">
                                        </div>
                                        <div class="form-group">
                                            <label for="editSourcePort">Source Port</label>
                                            <input type="number" id="editSourcePort" name="sourcePort" placeholder="6661" min="1" max="65535">
                                        </div>
                                    </div>
                                </div>
                            </div>

                            <!-- Target Configuration Section -->
                            <div class="config-section">
                                <h4 class="section-title">🎯 TARGET CONFIGURATION</h4>
                                <div class="form-row">
                                    <div class="form-group">
                                        <label for="editTargetType">Target Type</label>
                                        <select id="editTargetType" name="targetType" onchange="updateTargetFields()" required>
                                            <option value="fhir">FHIR Server</option>
                                            <option value="hl7">HL7 System</option>
                                            <option value="database">Database</option>
                                            <option value="file">File System</option>
                                            <option value="http">HTTP API</option>
                                            <option value="sink">Sink (Log Only)</option>
                                        </select>
                                    </div>

                                    <div class="form-group">
                                        <label for="editTargetConnectivity">Target Connectivity</label>
                                        <select id="editTargetConnectivity" name="targetConnectivity" required>
                                            <option value="outbound">Outbound (Send)</option>
                                            <option value="inbound">Inbound (Receive)</option>
                                            <option value="bidirectional">Bidirectional</option>
                                        </select>
                                    </div>
                                </div>

                                <!-- FHIR Server Configuration -->
                                <div id="fhirServerConfig">
                                    <div class="form-row">
                                        <div class="form-group">
                                            <label for="editFhirServerUrl">FHIR Server URL <span style="color: red;">*</span></label>
                                            <input type="text" id="editFhirServerUrl" name="fhirServerUrl" placeholder="http://localhost:8080/fhir" required>
                                            <small class="form-help">Full URL to FHIR server base</small>
                                        </div>
                                        <div class="form-group">
                                            <label for="editTargetPort">Port (if different)</label>
                                            <input type="number" id="editTargetPort" name="targetPort" placeholder="8080" min="1" max="65535">
                                        </div>
                                    </div>

                                    <div class="form-group">
                                        <label for="editResourceEndpoint">Resource Endpoint</label>
                                        <input type="text" id="editResourceEndpoint" name="resourceEndpoint" placeholder="Patient">
                                        <small class="form-help">FHIR resource endpoint (e.g., Patient, Observation)</small>
                                    </div>
                                </div>

                                <!-- Multiple Endpoints Configuration -->
                                <div class="form-group">
                                    <label class="checkbox-label">
                                        <input type="checkbox" id="enableMultipleEndpoints" name="enableMultipleEndpoints" onchange="toggleMultipleEndpoints()">
                                        <span class="checkmark"></span>
                                        Enable Multiple Endpoints
                                    </label>
                                    <small class="form-help">Send to multiple destinations with failover support</small>
                                </div>

                                <div id="multipleEndpointsConfig" style="display: none;">
                                    <div class="endpoints-container">
                                        <div class="endpoint-item" data-endpoint="0">
                                            <div class="endpoint-header">
                                                <h5>Primary Endpoint</h5>
                                                <button type="button" class="btn-remove-endpoint" onclick="removeEndpoint(0)" style="display: none;">Remove</button>
                                            </div>
                                            <div class="form-row">
                                                <div class="form-group">
                                                    <label>Endpoint Name</label>
                                                    <input type="text" name="endpointName_0" placeholder="Primary FHIR Server">
                                                </div>
                                                <div class="form-group">
                                                    <label>Base URL</label>
                                                    <input type="text" name="endpointUrl_0" placeholder="http://localhost:8081/fhir">
                                                </div>
                                            </div>
                                            <div class="form-row">
                                                <div class="form-group">
                                                    <label>Priority</label>
                                                    <select name="endpointPriority_0">
                                                        <option value="1">1 (Highest)</option>
                                                        <option value="2">2</option>
                                                        <option value="3">3 (Lowest)</option>
                                                    </select>
                                                </div>
                                                <div class="form-group">
                                                    <label class="checkbox-label">
                                                        <input type="checkbox" name="endpointEnabled_0" checked>
                                                        <span class="checkmark"></span>
                                                        Enabled
                                                    </label>
                                                </div>
                                            </div>
                                        </div>
                                    </div>
                                    <button type="button" class="btn-add-endpoint" onclick="addEndpoint()">+ Add Backup Endpoint</button>
                                </div>
                            </div>

                            <!-- Message Flow Configuration -->
                            <div class="config-section">
                                <h4 class="section-title">🔄 Message Flow & Routing</h4>

                                <div class="form-group">
                                    <label for="editRoutingMode">Routing Mode</label>
                                    <select id="editRoutingMode" name="routingMode" onchange="updateRoutingFields()">
                                        <option value="direct">Direct Processing</option>
                                        <option value="transform">Transform & Route</option>
                                        <option value="hl7-to-fhir">HL7 → FHIR Flow</option>
                                        <option value="fhir-to-hl7">FHIR → HL7 Flow</option>
                                    </select>
                                </div>

                                <!-- HL7 to FHIR Routing Configuration -->
                                <div id="hl7ToFhirConfig" style="display: none;">
                                    <div class="form-group">
                                        <label for="editTargetFhirInterface">Target FHIR Interface</label>
                                        <select id="editTargetFhirInterface" name="targetFhirInterface">
                                            <option value="">Select FHIR Interface...</option>
                                            <!-- Will be populated with available FHIR interfaces -->
                                        </select>
                                    </div>

                                    <div class="form-row">
                                        <div class="form-group">
                                            <label for="editTransformationEngine">Transformation Engine</label>
                                            <select id="editTransformationEngine" name="transformationEngine">
                                                <option value="go-engine">Go Engine (Default)</option>
                                                <option value="node-engine">Node.js Engine</option>
                                                <option value="external">External Service</option>
                                            </select>
                                        </div>

                                        <div class="form-group">
                                            <label for="editRetryPolicy">Retry Policy</label>
                                            <select id="editRetryPolicy" name="retryPolicy">
                                                <option value="3">3 attempts</option>
                                                <option value="5">5 attempts</option>
                                                <option value="10">10 attempts</option>
                                                <option value="none">No retries</option>
                                            </select>
                                        </div>
                                    </div>
                                </div>
                            </div>

                            <!-- Performance & Table Management -->
                            <div class="config-section">
                                <h4 class="section-title">⚡ Performance & Storage</h4>

                                <div class="form-row">
                                    <div class="form-group">
                                        <label for="editTableStrategy">Table Management</label>
                                        <select id="editTableStrategy" name="tableStrategy">
                                            <option value="shared">Shared Table (Default)</option>
                                            <option value="dedicated">Dedicated Table (High Performance)</option>
                                            <option value="hybrid">Hybrid Strategy</option>
                                        </select>
                                        <small class="form-help">Dedicated tables provide better performance for high-volume interfaces</small>
                                    </div>

                                    <div class="form-group">
                                        <label for="editExpectedVolume">Expected Message Volume</label>
                                        <select id="editExpectedVolume" name="expectedVolume">
                                            <option value="low">Low (< 1K/day)</option>
                                            <option value="medium">Medium (1K-50K/day)</option>
                                            <option value="high">High (50K+/day)</option>
                                        </select>
                                    </div>
                                </div>
                            </div>

                        </form>
                    </div>
                    <div class="modal-footer">
                        <button type="button" class="modal-btn secondary" onclick="closeEditModal()">Cancel</button>
                        <button type="button" class="modal-btn info" onclick="testInterfaceConfiguration()">🧪 Test Configuration</button>
                        <button type="submit" class="modal-btn primary" form="editInterfaceForm">💾 Save Changes</button>
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
        
        // ✅ PRESERVED: Details modal HTML with original IDs
        container.innerHTML = `
            <!-- Compact Interface Details Modal -->
            <div class="modal-overlay" id="detailsModal">
                <div class="modal-content large">
                    <div class="modal-header">
                        <h3 class="modal-title" id="detailsTitle">Interface Details</h3>
                        <button class="modal-close" onclick="closeDetailsModal()">&times;</button>
                    </div>
                    <div class="modal-body">
                        <div id="detailsContent">
                            <!-- Details populated by JavaScript -->
                        </div>
                    </div>
                    <div class="modal-footer">
                        <button type="button" class="modal-btn secondary" onclick="closeDetailsModal()">Close</button>
                    </div>
                </div>
            </div>
        `;
    }
    
    // ✅ Load components when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', loadModalComponents);
    } else {
        loadModalComponents();
    }
    
    console.log('✅ Modal components loaded');

    // Multiple Endpoints Management Functions
    window.toggleMultipleEndpoints = function() {
        const checkbox = document.getElementById('enableMultipleEndpoints');
        const config = document.getElementById('multipleEndpointsConfig');

        if (checkbox.checked) {
            config.style.display = 'block';
        } else {
            config.style.display = 'none';
        }
    };

    window.addEndpoint = function() {
        const container = document.querySelector('.endpoints-container');
        const endpointCount = container.children.length;

        const endpointHtml = `
            <div class="endpoint-item" data-endpoint="${endpointCount}">
                <div class="endpoint-header">
                    <h5>Backup Endpoint ${endpointCount}</h5>
                    <button type="button" class="btn-remove-endpoint" onclick="removeEndpoint(${endpointCount})">Remove</button>
                </div>
                <div class="form-row">
                    <div class="form-group">
                        <label>Endpoint Name</label>
                        <input type="text" name="endpointName_${endpointCount}" placeholder="Backup FHIR Server">
                    </div>
                    <div class="form-group">
                        <label>Base URL</label>
                        <input type="text" name="endpointUrl_${endpointCount}" placeholder="http://localhost:8082/fhir">
                    </div>
                </div>
                <div class="form-row">
                    <div class="form-group">
                        <label>Priority</label>
                        <select name="endpointPriority_${endpointCount}">
                            <option value="1">1 (Highest)</option>
                            <option value="2" selected>2</option>
                            <option value="3">3 (Lowest)</option>
                        </select>
                    </div>
                    <div class="form-group">
                        <label class="checkbox-label">
                            <input type="checkbox" name="endpointEnabled_${endpointCount}">
                            <span class="checkmark"></span>
                            Enabled
                        </label>
                    </div>
                </div>
            </div>
        `;

        container.insertAdjacentHTML('beforeend', endpointHtml);
    };

    window.removeEndpoint = function(endpointIndex) {
        const endpoint = document.querySelector(`[data-endpoint="${endpointIndex}"]`);
        if (endpoint && endpointIndex > 0) { // Don't allow removing primary endpoint
            endpoint.remove();
        }
    };

    // Update Target Fields based on Target Type selection
    window.updateTargetFields = function() {
        const targetType = document.getElementById('editTargetType').value;
        const fhirConfig = document.getElementById('fhirServerConfig');

        // Show/hide FHIR-specific fields based on target type
        if (targetType === 'fhir') {
            fhirConfig.style.display = 'block';
            document.getElementById('editFhirServerUrl').required = true;
        } else {
            fhirConfig.style.display = 'none';
            document.getElementById('editFhirServerUrl').required = false;
        }
    };

})();