// js/components/header-component.js - Interface Header Component Loader
// Loads the interface header with status tiles

(function() {
    'use strict';
    
    function loadHeaderComponent() {
        const container = document.getElementById('interface-header-container');
        if (!container) {
            console.warn('⚠️ Interface header container not found');
            return;
        }
        
        // ✅ PRESERVED: Header HTML with all original IDs and structure
        container.innerHTML = `
            <!-- Ultra Compact Header with Inline Status Tiles -->
            <header class="main-header">
                <div class="header-left">
                    <h1 class="page-title">Interfaces</h1>
                    
                    <!-- Status tiles inline in header -->
                    <div class="header-status-tiles">
                        <div class="status-tile">
                            <span class="status-tile-value" id="totalInterfaces">0</span>
                            <span class="status-tile-label">Total</span>
                            <span class="status-tile-icon">📊</span>
                        </div>
                        
                        <div class="status-tile">
                            <span class="status-tile-value" id="runningInterfaces">0</span>
                            <span class="status-tile-label">Running</span>
                            <span class="status-tile-icon">🟢</span>
                        </div>
                        
                        <div class="status-tile">
                            <span class="status-tile-value" id="stoppedInterfaces">0</span>
                            <span class="status-tile-label">Stopped</span>
                            <span class="status-tile-icon">🔴</span>
                        </div>
                        
                        <div class="status-tile">
                            <span class="status-tile-value" id="pausedInterfaces">0</span>
                            <span class="status-tile-label">Paused</span>
                            <span class="status-tile-icon">⏸️</span>
                        </div>
                        
                        <!-- Auto-refresh indicator in header -->
                        <div class="status-tile" id="autoRefreshIndicator">
                            <span class="status-tile-icon">🔄</span>
                            <span class="status-tile-label">Auto: <span id="refreshStatus">ON</span></span>
                        </div>
                        
                        <!-- Last updated in header -->
                        <div class="status-tile" id="timeDisplay">
                            <span class="status-tile-icon">🕐</span>
                            <span class="status-tile-label"><span id="currentTime">Loading...</span></span>
                        </div>
                    </div>
                </div>
                
                <div class="header-right">
                    <button class="create-btn" onclick="openInterfaceWizard()" id="createInterfaceBtn">
                        <span class="btn-icon">+</span>
                        <span class="btn-text">Create Interface</span>
                    </button>
                </div>
            </header>
        `;
        
        console.log('✅ Interface header component loaded');
    }
    
    // ✅ Load component when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', loadHeaderComponent);
    } else {
        loadHeaderComponent();
    }
    
})();