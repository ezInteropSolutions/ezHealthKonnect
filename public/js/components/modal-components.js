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
        
        // ✅ PRESERVED: Edit modal HTML with original IDs
        container.innerHTML = `
            <!-- Compact Edit Interface Modal -->
            <div class="modal-overlay" id="editModal">
                <div class="modal-content">
                    <div class="modal-header">
                        <h3 class="modal-title" id="editTitle">Edit Interface</h3>
                        <button class="modal-close" onclick="closeEditModal()">&times;</button>
                    </div>
                    <div class="modal-body">
                        <form id="editInterfaceForm">
                            <input type="hidden" id="editInterfaceId" name="id">
                            
                            <div class="form-group">
                                <label for="editInterfaceName">Interface Name</label>
                                <input type="text" id="editInterfaceName" name="name" required>
                            </div>
                            
                            <div class="form-group">
                                <label for="editInterfaceDescription">Description</label>
                                <textarea id="editInterfaceDescription" name="description"></textarea>
                            </div>

                            <div class="form-row">
                                <div class="form-group">
                                    <label for="editSourceType">Source Type</label>
                                    <select id="editSourceType" name="sourceType" required>
                                        <option value="file">File</option>
                                        <option value="tcp">TCP</option>
                                        <option value="http">HTTP</option>
                                        <option value="database">Database</option>
                                    </select>
                                </div>
                                
                                <div class="form-group">
                                    <label for="editTargetType">Target Type</label>
                                    <select id="editTargetType" name="targetType" required>
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
    
})();