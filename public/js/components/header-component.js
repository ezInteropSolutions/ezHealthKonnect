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
                            <i class="far fa-layer-group status-tile-icon"></i>
                            <span class="status-tile-value" id="totalInterfaces">0</span>
                            <span class="status-tile-label">Total</span>
                        </div>

                        <div class="status-tile">
                            <i class="far fa-circle status-tile-icon tile-icon-running"></i>
                            <span class="status-tile-value" id="runningInterfaces">0</span>
                            <span class="status-tile-label">Running</span>
                        </div>

                        <div class="status-tile">
                            <i class="far fa-circle status-tile-icon tile-icon-stopped"></i>
                            <span class="status-tile-value" id="stoppedInterfaces">0</span>
                            <span class="status-tile-label">Stopped</span>
                        </div>

                        <div class="status-tile">
                            <i class="far fa-pause-circle status-tile-icon tile-icon-paused"></i>
                            <span class="status-tile-value" id="pausedInterfaces">0</span>
                            <span class="status-tile-label">Paused</span>
                        </div>

                        <!-- Auto-refresh indicator -->
                        <div class="status-tile" id="autoRefreshIndicator">
                            <i class="far fa-clock status-tile-icon tile-icon-auto"></i>
                            <span class="status-tile-label">Auto: <span id="refreshStatus">ON</span></span>
                        </div>

                        <!-- Clock -->
                        <div class="status-tile" id="timeDisplay">
                            <i class="far fa-clock status-tile-icon"></i>
                            <span class="status-tile-label"><span id="currentTime">Loading...</span></span>
                        </div>
                    </div>
                </div>
                
                <div class="header-right">
                    <button class="create-btn" onclick="openWizard()" id="createInterfaceBtn">
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