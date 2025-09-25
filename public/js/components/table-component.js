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
            <!-- Processing Engine Control Panel -->
            <div class="engine-control-panel" id="engine-control-panel">
                <div class="engine-status">
                    <div class="engine-indicator">
                        <span class="engine-light" id="engine-light">●</span>
                        <span class="engine-text" id="engine-text">Processing Engine: Checking...</span>
                    </div>
                    <div class="engine-controls">
                        <button class="engine-btn start" id="start-engine-btn" onclick="startProcessingEngine()">
                            🚀 Start Engine
                        </button>
                        <button class="engine-btn stop" id="stop-engine-btn" onclick="stopProcessingEngine()" style="display:none">
                            ⏹️ Stop Engine
                        </button>
                    </div>
                </div>
                <div class="engine-stats" id="engine-stats">
                    <div class="stat-item">
                        <span class="stat-label">Active Interfaces:</span>
                        <span class="stat-value" id="active-interfaces-count">0</span>
                    </div>
                    <div class="stat-item">
                        <span class="stat-label">Messages Today:</span>
                        <span class="stat-value" id="messages-today-count">0</span>
                    </div>
                    <div class="stat-item">
                        <span class="stat-label">Success Rate:</span>
                        <span class="stat-value" id="success-rate">0%</span>
                    </div>
                </div>
            </div>

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
                                <th>Config Status</th>
                                <th>Runtime Status</th>
                                <th>Last Updated</th>
                                <th>Statistics</th>
                                <th>Last Activity</th>
                                <th>Actions</th>
                            </tr>
                        </thead>
                        <tbody id="interfacesTableBody">
                            <!-- Ultra compact table rows populated by JavaScript -->
                            <tr>
                                <td colspan="7">
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