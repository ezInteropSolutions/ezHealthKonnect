// js/components/table-component.js - Interfaces Table Component Loader
// Loads the interfaces table section

(function() {
    'use strict';
    
    function loadTableComponent() {
        const container = document.getElementById('interfaces-table-container');
        if (!container) {
            console.warn('⚠️ Interfaces table container not found');
            return;
        }
        
        // ✅ PRESERVED: Table HTML with all original IDs and structure
        container.innerHTML = `
            <!-- Ultra Compact Interface Table - No Header Text -->
            <div class="interfaces-section">
                <div class="section-header">
                    <!-- Only filters, no title text -->
                    <div class="section-filters">
                        <select class="filter-select" id="statusFilter">
                            <option value="all">All Status</option>
                            <option value="running">Running</option>
                            <option value="stopped">Stopped</option>
                            <option value="paused">Paused</option>
                            <option value="error">Error</option>
                        </select>
                        <select class="filter-select" id="typeFilter">
                            <option value="all">All Types</option>
                            <option value="file">File</option>
                            <option value="tcp">TCP</option>
                            <option value="http">HTTP</option>
                        </select>
                    </div>
                </div>

                <div class="table-container">
                    <table class="interfaces-table">
                        <thead>
                            <tr>
                                <th>Interface Name</th>
                                <th>Status</th>
                                <th>Last Updated</th>
                                <th>Statistics</th>
                                <th>Last Activity</th>
                                <th>Actions</th>
                            </tr>
                        </thead>
                        <tbody id="interfacesTableBody">
                            <!-- Ultra compact table rows populated by JavaScript -->
                            <tr>
                                <td colspan="6">
                                    <div class="loading-card">
                                        <div class="loading-spinner"></div>
                                        <p>Loading interfaces...</p>
                                    </div>
                                </td>
                            </tr>
                        </tbody>
                    </table>

                    <!-- Ultra Compact Pagination -->
                    <div class="table-footer">
                        <div class="pagination-info">Loading...</div>
                        
                        <div style="display: flex; align-items: center; gap: 6px;">
                            <div class="page-size-selector">
                                <span>Show:</span>
                                <select class="page-size-select" id="pageSize">
                                    <option value="10">10</option>
                                    <option value="25" selected>25</option>
                                    <option value="50">50</option>
                                    <option value="100">100</option>
                                </select>
                            </div>
                            
                            <div class="pagination-controls">
                                <!-- Mini pagination buttons populated by JavaScript -->
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        `;
        
        console.log('✅ Interfaces table component loaded');
    }
    
    // ✅ Load component when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', loadTableComponent);
    } else {
        loadTableComponent();
    }
    
})();