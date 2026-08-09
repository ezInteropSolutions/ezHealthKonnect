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
                        <span class="engine-light" id="engine-light"><i class="far fa-circle"></i></span>
                        <span class="engine-text" id="engine-text">Processing Engine: Checking...</span>
                    </div>
                    <div class="engine-controls">
                        <button class="engine-btn start" id="start-engine-btn" onclick="startProcessingEngine()">
                            <i class="far fa-play-circle"></i> Start Engine
                        </button>
                        <button class="engine-btn stop" id="stop-engine-btn" onclick="stopProcessingEngine()" style="display:none">
                            <i class="far fa-stop-circle"></i> Stop Engine
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
                    <div class="section-filters">
                        <!-- Search by name -->
                        <div class="search-input-wrapper" style="position:relative;">
                            <i class="fas fa-search" style="position:absolute;left:10px;top:50%;transform:translateY(-50%);color:#94a3b8;font-size:0.8rem;pointer-events:none;"></i>
                            <input type="text" id="interfaceSearchInput" placeholder="Search interfaces…" style="
                                padding: 0.4rem 0.75rem 0.4rem 2rem;
                                border: 1px solid #e2e8f0;
                                border-radius: 6px;
                                font-size: 0.85rem;
                                width: 220px;
                                background: #fff;
                                color: #1e293b;
                                outline: none;
                                transition: border-color 0.15s;
                            " onfocus="this.style.borderColor='#3b82f6'" onblur="this.style.borderColor='#e2e8f0'">
                        </div>

                        <!-- Status filter -->
                        <select class="filter-select" id="statusFilter">
                            <option value="all">All Status</option>
                            <option value="active">Active</option>
                            <option value="running">Running</option>
                            <option value="stopped">Stopped</option>
                            <option value="paused">Paused</option>
                            <option value="error">Error</option>
                            <option value="inactive">Inactive</option>
                            <option value="configured">Configured</option>
                            <option value="draft">Draft</option>
                        </select>

                        <!-- Type filter -->
                        <select class="filter-select" id="typeFilter">
                            <option value="all">All Types</option>
                            <option value="hl7">HL7</option>
                            <option value="fhir">FHIR</option>
                            <option value="database">Database</option>
                            <option value="file">File</option>
                            <option value="tcp">TCP/MLLP</option>
                            <option value="http">HTTP</option>
                        </select>

                        <!-- Inbound source format filter — matches sourceType (source_type in DB) -->
                        <select class="filter-select" id="messageTypeFilter">
                            <option value="all">All Formats</option>
                            <option value="hl7v2">HL7 v2.x (ADT, ORU…)</option>
                            <option value="fhir">FHIR (R4 / STU3)</option>
                            <option value="cda">CDA / C-CDA</option>
                            <option value="x12">X12 (Claims)</option>
                            <option value="json">JSON</option>
                            <option value="xml">XML</option>
                            <option value="csv">CSV</option>
                        </select>
                    </div>
                </div>

                <div class="table-container">
                    <table class="interfaces-table">
                        <thead>
                            <tr>
                                <th class="th-sortable" onclick="sortBy('name')" title="Interface Name">Interface Name <span class="sort-arrow" id="sort-arrow-name">↕</span></th>
                                <th class="th-sortable" onclick="sortBy('status')" title="Config Status">Config <span class="sort-arrow" id="sort-arrow-status">↕</span></th>
                                <th class="th-sortable" onclick="sortBy('runtime')" title="Runtime Status">Runtime <span class="sort-arrow" id="sort-arrow-runtime">↕</span></th>
                                <th class="th-sortable" onclick="sortBy('created')" title="Created">Created <span class="sort-arrow" id="sort-arrow-created">↕</span></th>
                                <th class="th-sortable" onclick="sortBy('lastUpdated')" title="Last Updated">Last Updated <span class="sort-arrow" id="sort-arrow-lastUpdated">↕</span></th>
                                <th class="th-sortable" onclick="sortBy('statistics')" title="Statistics">Statistics <span class="sort-arrow" id="sort-arrow-statistics">↕</span></th>
                                <th class="th-sortable" onclick="sortBy('lastActivity')" title="Last Activity">Activity <span class="sort-arrow" id="sort-arrow-lastActivity">↕</span></th>
                                <th title="Actions">Actions</th>
                            </tr>
                        </thead>
                        <tbody id="interfacesTableBody">
                            <!-- Ultra compact table rows populated by JavaScript -->
                            <tr>
                                <td colspan="8">
                                    <div class="loading-card">
                                        <div class="loading-spinner"></div>
                                        <p>Loading interfaces...</p>
                                    </div>
                                </td>
                            </tr>
                        </tbody>
                    </table>
                </div>

                <!-- Pagination Footer — outside table-container so it never scrolls away -->
                <div class="table-footer" style="display:flex;align-items:center;justify-content:space-between;padding:0.6rem 1rem;border-top:1px solid #e5e7eb;background:#f8fafc;flex-wrap:wrap;gap:0.5rem;flex-shrink:0;">
                    <div class="pagination-info" style="color:#64748b;font-size:0.8rem;"></div>

                    <div style="display:flex;align-items:center;gap:0.75rem;">
                        <!-- Page size -->
                        <div style="display:flex;align-items:center;gap:0.4rem;color:#64748b;font-size:0.8rem;">
                            <span>Show</span>
                            <select id="pageSize" style="padding:0.25rem 0.4rem;border:1px solid #e2e8f0;border-radius:4px;font-size:0.8rem;background:#fff;">
                                <option value="10">10</option>
                                <option value="25" selected>25</option>
                                <option value="50">50</option>
                                <option value="100">100</option>
                            </select>
                            <span>per page</span>
                        </div>

                        <!-- Pagination controls -->
                        <div class="pagination-controls" style="display:flex;align-items:center;gap:3px;">
                            <!-- Populated by JavaScript -->
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